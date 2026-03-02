package mcp

import (
	"context"
	"fmt"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/urugus/second-brain/internal/kb"
)

type kbReadInput struct {
	Path string `json:"path" jsonschema:"Relative path to the KB file"`
}

type kbSearchInput struct {
	Query string `json:"query" jsonschema:"Search query (case-insensitive substring match)"`
}

type kbWriteInput struct {
	Path    string `json:"path" jsonschema:"Relative path for the KB file (e.g. topic/name.md)"`
	Content string `json:"content" jsonschema:"Complete markdown content to write"`
}

func registerKBTools(server *gomcp.Server, k *kb.KB) {
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "kb_list",
		Description: "List all knowledge base files",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, input struct{}) (*gomcp.CallToolResult, any, error) {
		files, err := k.List()
		if err != nil || len(files) == 0 {
			return textResult("No KB files found"), nil, nil
		}
		r, err := jsonResult(files)
		return r, nil, err
	})

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "kb_read",
		Description: "Read a knowledge base file by its relative path",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, input kbReadInput) (*gomcp.CallToolResult, any, error) {
		content, err := k.Read(input.Path)
		if err != nil {
			return errResult("failed to read KB file: " + err.Error()), nil, nil
		}
		return textResult(content), nil, nil
	})

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "kb_search",
		Description: "Search knowledge base files for a query string (case-insensitive)",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, input kbSearchInput) (*gomcp.CallToolResult, any, error) {
		results, err := k.Search(input.Query)
		if err != nil {
			return errResult("failed to search KB: " + err.Error()), nil, nil
		}
		if len(results) == 0 {
			return textResult("No search results found"), nil, nil
		}
		r, err := jsonResult(results)
		return r, nil, err
	})

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "kb_write",
		Description: "Write or update a knowledge base file. Creates parent directories as needed.",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, input kbWriteInput) (*gomcp.CallToolResult, any, error) {
		err := k.Write(input.Path, input.Content)
		if err != nil {
			return errResult("failed to write KB file: " + err.Error()), nil, nil
		}
		return textResult(fmt.Sprintf("KB file %q written successfully", input.Path)), nil, nil
	})
}
