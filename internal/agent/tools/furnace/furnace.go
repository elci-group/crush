package furnace

import (
	"cmp"
	"context"
	_ "embed"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/filepathext"
	"github.com/charmbracelet/crush/internal/fsext"
	"github.com/charmbracelet/crush/internal/permission"
)

type FurnaceParams struct {
	Path   string `json:"path,omitempty" description:"The path to the Rust project to scan (defaults to current working directory)"`
	Format string `json:"format,omitempty" description:"Output format: 'text' or 'json' (default is 'text')"`
}

type FurnacePermissionsParams struct {
	Path   string `json:"path"`
	Format string `json:"format"`
}

const (
	FurnaceToolName = "furnace"
)

//go:embed furnace.md
var furnaceDescription []byte

func NewFurnaceTool(permissions permission.Service, workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		FurnaceToolName,
		string(furnaceDescription),
		func(ctx context.Context, params FurnaceParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			searchPath, err := fsext.Expand(cmp.Or(params.Path, workingDir))
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("error expanding path: %v", err)), nil
			}

			searchPath = filepathext.SmartJoin(workingDir, searchPath)

			// Check if path is outside working directory and request permission if needed
			absWorkingDir, err := filepath.Abs(workingDir)
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("error resolving working directory: %v", err)), nil
			}

			absSearchPath, err := filepath.Abs(searchPath)
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("error resolving search path: %v", err)), nil
			}

			relPath, err := filepath.Rel(absWorkingDir, absSearchPath)
			if err != nil || strings.HasPrefix(relPath, "..") {
				sessionID := tools.GetSessionFromContext(ctx)
				if sessionID == "" {
					return fantasy.ToolResponse{}, fmt.Errorf("session ID is required for accessing directories outside working directory")
				}

				granted, err := permissions.Request(ctx,
					permission.CreatePermissionRequest{
						SessionID:   sessionID,
						Path:        absSearchPath,
						ToolCallID:  call.ID,
						ToolName:    FurnaceToolName,
						Action:      "scan",
						Description: fmt.Sprintf("Scan Rust project outside working directory: %s", absSearchPath),
						Params:      FurnacePermissionsParams(params),
					},
				)
				if err != nil {
					return fantasy.ToolResponse{}, err
				}
				if !granted {
					return fantasy.ToolResponse{}, permission.ErrorPermissionDenied
				}
			}

			// Execute furnace command
			format := cmp.Or(params.Format, "text")
			args := []string{searchPath}
			if format == "json" {
				args = append(args, "--format", "json")
			}

			cmd := exec.CommandContext(ctx, "furnace", args...)
			output, err := cmd.CombinedOutput()
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("furnace failed: %v\nOutput: %s", err, string(output))), nil
			}

			return fantasy.NewTextResponse(string(output)), nil
		})
}
