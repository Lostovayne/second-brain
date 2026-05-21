# Second Brain MCP

A lightweight, high-performance Model Context Protocol (MCP) server designed to act as a local semantic memory and code structure harvester for AI assistants.

## Purpose

The project aims to optimize AI context window consumption by offering a fine-grained, localized repository of knowledge and architectural insights. By structuring relational observations in a local SQLite database, it eliminates the need to feed large codebases or heavy document blocks into the AI context window, drastically reducing token usage while preserving reasoning quality.

## Key Benefits

- **Token Conservation**: Provides granular context on-demand rather than dumping large files or directory listings into the context window.
- **Minimal Footprint**: Implements full-text search with natural relevance ranking (BM25) over memory structures, avoiding the overhead of heavy vector databases.
- **Relational Logic**: Infers non-obvious, multi-hop architectural dependencies directly at the database level.
- **Zero Overhead Development**: Performs directory walking and Go AST parsing in Go native code with sub-30ms execution times, allowing trigger-based reindexing instead of running heavy background file watchers.
- **Contextual Scoping**: Dynamically isolates observations by project directory while supporting global rules and general patterns, allowing seamless knowledge inheritance.

## Core Capabilities

### 1. Hierarchical Isolation Scoping
All observations, entities, and relations are partitioned using a project identifier. When a search is performed, the query retrieves the union of the active project and the global namespace. This allows general coding guidelines and architecture rules to be inherited across all workspaces without cross-contaminating project-specific memories.

### 2. AST-Based Project Structure Harvester
A lightweight Go AST parser inspects directory trees to map the exact physical and syntactic topology of the codebase. It parses `.go` files on the fly to extract public Structs, Interfaces, and Functions (including receivers and signatures). The harvester automatically ignores configurations, logs, databases, and build artifacts (such as `.git`, `node_modules`, `vendor`, `.atl`, `dist`, `build`, `bin`, `.vscode`, `.idea`).

### 3. Transitive Graph Inference Engine
Using SQLite recursive Common Table Expressions (CTEs), the system traverses semantic relations up to 3 hops. This enables the engine to resolve transitive paths (for example, if `A` uses `B`, and `B` is deprecated by `C`, the system infers that `A` is affected by the deprecation of `C`) and injects these deductions into the AI context.

## Compilation and Deployment

If you prefer to build the standalone binary yourself instead of using precompiled release assets, follow this step-by-step guide.

### Prerequisites

Ensure you have Go 1.21 or higher installed. You can verify your installation by running:

```bash
go version
```

### Step 1: Compile the Executable

Run the compilation command from the root of the repository. This will compile the codebase and place the executable inside a dedicated `bin` directory:

#### Windows
```powershell
go build -o bin/mcp-server.exe ./cmd/mcp-server
```

#### Linux and macOS
```bash
go build -o bin/mcp-server ./cmd/mcp-server
```

### Step 2: Relocate the Binary

Move the compiled executable (`mcp-server.exe` on Windows or `mcp-server` on Unix-like systems) to a stable, dedicated directory on your system where it will not be modified or deleted by cleanups (for example, `C:/mcp-servers/second-brain/` or `/usr/local/bin/`).

### Step 3: Register the Absolute Path

Identify the absolute path where you placed the binary. You will need to provide this exact path when configuring the Model Context Protocol in the next section.

## Model Context Protocol (MCP) Registration

To integrate the server with your MCP client (such as Claude Desktop or Antigravity), register the compiled binary in your client's configuration file:

### Configuration Template

```json
{
  "mcpServers": {
    "second-brain": {
      "type": "stdio",
      "command": "C:/absolute/path/to/second-brain/mcp-server.exe",
      "args": []
    }
  }
}
```

Ensure the path to the executable is absolute and uses forward slashes `/` to prevent escape character issues.
