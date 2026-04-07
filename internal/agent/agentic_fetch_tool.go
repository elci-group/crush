package agent

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/agent/prompt"
	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/permission"
)

//go:embed templates/agentic_fetch.md
var agenticFetchToolDescription []byte

// agenticFetchValidationResult holds the validated parameters from the tool call context.
type agenticFetchValidationResult struct {
	SessionID      string
	AgentMessageID string
}

// validateAgenticFetchParams validates the tool call parameters and extracts required context values.
func validateAgenticFetchParams(ctx context.Context, params tools.AgenticFetchParams) (agenticFetchValidationResult, error) {
	if params.Prompt == "" {
		return agenticFetchValidationResult{}, errors.New("prompt is required")
	}

	sessionID := tools.GetSessionFromContext(ctx)
	if sessionID == "" {
		return agenticFetchValidationResult{}, errors.New("session id missing from context")
	}

	agentMessageID := tools.GetMessageFromContext(ctx)
	if agentMessageID == "" {
		return agenticFetchValidationResult{}, errors.New("agent message id missing from context")
	}

	return agenticFetchValidationResult{
		SessionID:      sessionID,
		AgentMessageID: agentMessageID,
	}, nil
}

//go:embed templates/agentic_fetch_prompt.md.tpl
var agenticFetchPromptTmpl []byte

func (c *coordinator) agenticFetchTool(_ context.Context, client *http.Client) (fantasy.AgentTool, error) {
	if client == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.MaxIdleConns = 100
		transport.MaxIdleConnsPerHost = 10
		transport.IdleConnTimeout = 90 * time.Second

		client = &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		}
	}

	return fantasy.NewParallelAgentTool(
		tools.AgenticFetchToolName,
		string(agenticFetchToolDescription),
		func(ctx context.Context, params tools.AgenticFetchParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			validationResult, err := validateAgenticFetchParams(ctx, params)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			// Determine description based on mode.
			var description string
			if params.URL != "" {
				description = fmt.Sprintf("Fetch and analyze content from URL: %s", params.URL)
			} else {
				description = "Search the web and analyze results"
			}

			p, err := c.permissions.Request(ctx,
				permission.CreatePermissionRequest{
					SessionID:   validationResult.SessionID,
					Path:        c.cfg.WorkingDir(),
					ToolCallID:  call.ID,
					ToolName:    tools.AgenticFetchToolName,
					Action:      "fetch",
					Description: description,
					Params:      tools.AgenticFetchPermissionsParams(params),
				},
			)
			if err != nil {
				return fantasy.ToolResponse{}, err
			}
			if !p {
				return fantasy.ToolResponse{}, permission.ErrorPermissionDenied
			}

			tmpDir, err := os.MkdirTemp(c.cfg.Config().Options.DataDirectory, "crush-fetch-*")
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("Failed to create temporary directory: %s", err)), nil
			}
			defer os.RemoveAll(tmpDir)

			var fullPrompt string

			if params.URL != "" {
				// URL mode: fetch the URL content first.
				content, err := tools.FetchURLAndConvert(ctx, client, params.URL)
				if err != nil {
					return fantasy.NewTextErrorResponse(fmt.Sprintf("Failed to fetch URL: %s", err)), nil
				}

				hasLargeContent := len(content) > tools.LargeContentThreshold

				if hasLargeContent {
					tempFile, err := os.CreateTemp(tmpDir, "page-*.md")
					if err != nil {
						return fantasy.NewTextErrorResponse(fmt.Sprintf("Failed to create temporary file: %s", err)), nil
					}
					tempFilePath := tempFile.Name()

					if _, err := tempFile.WriteString(content); err != nil {
						tempFile.Close()
						return fantasy.NewTextErrorResponse(fmt.Sprintf("Failed to write content to file: %s", err)), nil
					}
					tempFile.Close()

					fullPrompt = fmt.Sprintf("%s\n\nThe web page from %s has been saved to: %s\n\nUse the view and grep tools to analyze this file and extract the requested information.", params.Prompt, params.URL, tempFilePath)
				} else {
					fullPrompt = fmt.Sprintf("%s\n\nWeb page URL: %s\n\n<webpage_content>\n%s\n</webpage_content>", params.Prompt, params.URL, content)
				}
			} else {
				// Search mode: let the sub-agent search and fetch as needed.
				fullPrompt = fmt.Sprintf("%s\n\nUse the web_search tool to find relevant information. Break down the question into smaller, focused searches if needed. After searching, use web_fetch to get detailed content from the most relevant results.", params.Prompt)
			}

			promptOpts := []prompt.Option{
				prompt.WithWorkingDir(tmpDir),
			}

			promptTemplate, err := prompt.NewPrompt("agentic_fetch", string(agenticFetchPromptTmpl), promptOpts...)
			if err != nil {
				slog.Error("Failed to create agentic fetch prompt template", "error", err)
				return fantasy.ToolResponse{}, fmt.Errorf("error creating prompt: %s", err)
			}

			// 1. Build agent models with comprehensive error checking
			slog.Debug("Building models for agentic fetch", "mode", "sub-agent")
			_, small, err := c.buildAgentModels(ctx, true)
			if err != nil {
				slog.Error("Failed to build agent models for agentic fetch", "error", err)
				return fantasy.ToolResponse{}, fmt.Errorf("error building models: %s", err)
			}

			if small.Model == nil {
				slog.Error("Small model is nil after buildAgentModels")
				return fantasy.ToolResponse{}, errors.New("small model is nil")
			}

			if small.ModelCfg.Provider == "" {
				slog.Error("Small model provider is empty")
				return fantasy.ToolResponse{}, errors.New("small model provider not set")
			}

			slog.Debug("Built models successfully", "small_model", small.ModelCfg.Model, "provider", small.ModelCfg.Provider)

			// 2. Build system prompt with detailed error handling
			slog.Debug("Building system prompt", "provider", small.Model.Provider(), "model", small.Model.Model())
			systemPrompt, err := promptTemplate.Build(ctx, small.Model.Provider(), small.Model.Model(), c.cfg)
			if err != nil {
				slog.Error("Failed to build system prompt", "provider", small.Model.Provider(), "model", small.Model.Model(), "error", err)
				return fantasy.ToolResponse{}, fmt.Errorf("error building system prompt: %s", err)
			}

			if systemPrompt == "" {
				slog.Error("System prompt is empty after build")
				return fantasy.ToolResponse{}, errors.New("system prompt is empty")
			}

			slog.Debug("System prompt built successfully", "length", len(systemPrompt))

			// 3. Verify provider configuration exists
			smallProviderCfg, ok := c.cfg.Config().Providers.Get(small.ModelCfg.Provider)
			if !ok {
				slog.Error("Small model provider not found in config", "provider", small.ModelCfg.Provider)
				return fantasy.ToolResponse{}, fmt.Errorf("small model provider '%s' not configured", small.ModelCfg.Provider)
			}

			slog.Debug("Provider config found", "provider", small.ModelCfg.Provider)

			// 4. Create fetch tools with proper initialization
			slog.Debug("Creating fetch tools")
			webFetchTool := tools.NewWebFetchTool(tmpDir, client)
			webSearchTool := tools.NewWebSearchTool(client)
			fetchTools := []fantasy.AgentTool{
				webFetchTool,
				webSearchTool,
				tools.NewGlobTool(tmpDir),
				tools.NewGrepTool(tmpDir, c.cfg.Config().Tools.Grep),
				tools.NewSourcegraphTool(client),
				tools.NewViewTool(c.lspManager, c.permissions, c.filetracker, tmpDir),
			}

			slog.Debug("Created fetch tools", "count", len(fetchTools))

			// 5. Create session agent with comprehensive logging
			slog.Debug("Creating session agent",
				"large_model", small.ModelCfg.Model,
				"small_model", small.ModelCfg.Model,
				"tools_count", len(fetchTools),
				"disable_auto_summarize", c.cfg.Config().Options.DisableAutoSummarize,
			)

			agent := NewSessionAgent(SessionAgentOptions{
				LargeModel:           small, // Use small model for both (fetch doesn't need large)
				SmallModel:           small,
				SystemPromptPrefix:   smallProviderCfg.SystemPromptPrefix,
				SystemPrompt:         systemPrompt,
				DisableAutoSummarize: c.cfg.Config().Options.DisableAutoSummarize,
				IsYolo:               c.permissions.SkipRequests(),
				Sessions:             c.sessions,
				Messages:             c.messages,
				Tools:                fetchTools,
			})

			if agent == nil {
				slog.Error("Failed to create session agent - agent is nil")
				return fantasy.ToolResponse{}, errors.New("failed to create session agent")
			}

			slog.Debug("Session agent created successfully")

			// 6. Run sub-agent with context
			slog.Debug("Running agentic fetch sub-agent",
				"session_id", validationResult.SessionID,
				"agent_message_id", validationResult.AgentMessageID,
				"tool_call_id", call.ID,
			)

			result, err := c.runSubAgent(ctx, subAgentParams{
				Agent:          agent,
				SessionID:      validationResult.SessionID,
				AgentMessageID: validationResult.AgentMessageID,
				ToolCallID:     call.ID,
				Prompt:         fullPrompt,
				SessionTitle:   "Fetch Analysis",
				SessionSetup: func(sessionID string) {
					c.permissions.AutoApproveSession(sessionID)
				},
			})

			if err != nil {
				slog.Error("Agentic fetch sub-agent failed", "error", err)
				return fantasy.ToolResponse{}, err
			}

			slog.Debug("Agentic fetch sub-agent completed successfully")
			return result, nil
		}), nil
}
