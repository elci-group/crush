# Kaptaind-Crush Architecture

## Project Overview

Kaptaind-Crush is a fork and advanced evolution of Crush, a terminal-based AI coding assistant. It retains the core philosophy of a session-based, Multi-Model, TUI-driven application while significantly expanding context awareness, autonomy, and external integrations.

## Core Architecture

The core architecture follows the stock Crush layout, built in Go with `charm.land/bubbletea/v2` for the UI and `charm.land/fantasy` for provider abstractions. 

### Key Components

- **`internal/app/`**: Top-level wiring linking database (SQLite via `sqlc`), configuration, agents, LSPs, MCPs, and pub/sub events.
- **`internal/agent/`**: Manages LLM conversations. The `SessionAgent` handles user interactions, while the `Coordinator` manages named agents like "coder" and "task" that work autonomously.
- **`internal/ui/`**: TUI elements built with Bubble Tea.
- **`internal/lsp/`**: Language Server Protocol client manager for in-context diagnostics and code intelligence.
- **`internal/db/`**: SQLite database managing chat history, sessions, and configuration persistence.
- **`internal/skills/`**: Dynamic loading of `.md` skills files to augment agent capabilities.

## Divergence from Stock Crush

Kaptaind-Crush introduces several major architectural shifts and additions designed to scale the application from a simple conversational assistant to a fully autonomous, context-aware coding agent.

### 1. Bound MCP Integration (Omniscient Context)
Unlike stock Crush, which relies primarily on iterative file reading and simple glob/grep tools, Kaptaind-Crush integrates a custom Rust-based tool called **`bound`** via the Model Context Protocol (MCP).

*   **Architecture:** `bound` is run as a separate Node.js MCP server (`bound-mcp`) that spawns the compiled Rust binary. It communicates with Kaptaind-Crush via `stdio`.
*   **Capabilities:** It provides recursive file aggregation, AST-aware dependency resolution (`{.ext}`), Furnace AST reporting, and native token/size/depth limit enforcement.
*   **Why it's different:** Stock Crush sends the LLM hunting for files one by one. Kaptaind-Crush uses `bound` to instantly package complete modules and their dependencies into a single, token-budgeted prompt injection.

### 2. Automatic Task Delegation
Kaptaind-Crush implements intelligent delegatability scoring on every user task. 
*   High-scoring tasks are automatically decomposed into sub-tasks.
*   The system spawns isolated git branches for these sub-tasks, delegates them to specialized agent models (self-calibrating based on task complexity), and manages the merge process.

### 3. Smart Context Injection & Chunking
*   **Multi-resolution Context:** Instead of sending raw files, the system can inject tree summaries, file summaries, or full content based on token availability.
*   **Intelligent Text Chunking:** For files exceeding model context windows, Kaptaind-Crush automatically splits the text with contextual headers and footers to maintain semantic continuity across LLM requests.

### 4. Multimedia & Accessibility Enhancements
*   **YouTube Integration:** Built-in mini-player allowing developers to watch coding tutorials or listen to audio (via `mpv`) natively inside the TUI.
*   **Speech Features:** Native TTS (Text-to-Speech) for having code read aloud and STT (Speech-to-Text) for hands-free prompting and command execution.

## Future Direction
Kaptaind-Crush will continue to leverage the MCP architecture to add powerful, language-specific out-of-process analyzers (like `bound` for Rust) without compromising the Go-native, zero-CGO philosophy of the core binary.