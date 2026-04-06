# Furnace Tool

The Furnace tool is a high-performance Rust project analyzer that provides structured semantic snapshots of your codebase. It uses the `furnace` CLI to scan Rust projects and extract detailed information about functions, structs, traits, and enums.

<usage>
- Provide the directory path of the Rust project to scan (defaults to the current working directory).
- Optionally specify an output format (text or json).
- The tool returns a structured overview of the project's architecture and code elements.
</usage>

<features>
- Semantic AST-based parsing of Rust files.
- Extraction of function signatures, struct fields, and trait definitions.
- Identification of module hierarchies and project structure.
- Support for multiple output styles for better human readability or machine processing.
</features>

<limitations>
- Currently only supports Rust projects with a valid `Cargo.toml`.
- Large projects may take a few seconds to scan.
</limitations>

<tips>
- Use this tool when you need to understand the architecture of a Rust project you haven't seen before.
- The JSON output is useful for programmatic analysis or when you need deep details about specific code items.
- Combine with the Grep tool to find specific implementations after identifying them with Furnace.
</tips>
