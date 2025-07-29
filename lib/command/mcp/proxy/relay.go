package mcpProxy

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/superfly/flyctl/lib/buildinfo"
)

func Replay(ctx context.Context, proxyInfo ProxyInfo) error {
	// Create a new MCP client based on the provided configuration
	mcpClient, err := newMCPClient(proxyInfo)
	if err != nil {
		return fmt.Errorf("error creating MCP client: %w", err)
	}
	defer mcpClient.Close()

	// Create a new MCP server
	mcpServer := server.NewMCPServer(
		"FlyMCP Proxy 🚀",
		buildinfo.Info().Version.String(),
	)

	// Add the MCP client to the server
	err = addToMCPServer(ctx, mcpClient, mcpServer)
	if err != nil {
		return fmt.Errorf("error adding MCP client to server: %w", err)
	}

	if proxyInfo.Ping {
		go startPingTask(ctx, mcpClient)
	}

	// Start the stdio server
	if err := server.ServeStdio(mcpServer); err != nil {
		return fmt.Errorf("error starting stdio server: %w", err)
	}

	return nil
}

func newMCPClient(proxyInfo ProxyInfo) (*client.Client, error) {
	headers := make(map[string]string)

	if proxyInfo.BearerToken != "" {
		headers["Authorization"] = "Bearer " + proxyInfo.BearerToken
	} else if proxyInfo.User != "" {
		auth := proxyInfo.User + ":" + proxyInfo.Password
		headers["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
	}

	if proxyInfo.Instance != "" {
		headers["Fly-Force-Instance-Id"] = proxyInfo.Instance
	}

	var err error
	var mcpClient *client.Client

	if proxyInfo.Mode == "sse" {
		var options []transport.ClientOption

		if len(headers) > 0 {
			options = append(options, client.WithHeaders(headers))
		}

		mcpClient, err = client.NewSSEMCPClient(proxyInfo.Url, options...)
	} else {
		var options []transport.StreamableHTTPCOption

		if len(headers) > 0 {
			options = append(options, transport.WithHTTPHeaders(headers))
		}

		if proxyInfo.Timeout > 0 {
			options = append(options, transport.WithHTTPTimeout(time.Duration(proxyInfo.Timeout)*time.Second))
		}

		mcpClient, err = client.NewStreamableHttpClient(proxyInfo.Url, options...)
	}

	if err != nil {
		return nil, err
	}

	return mcpClient, nil
}

func addToMCPServer(ctx context.Context, mcpClient *client.Client, mcpServer *server.MCPServer) error {
	err := mcpClient.Start(ctx)
	if err != nil {
		return err
	}

	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "FlyMCP Proxy Client",
		Version: buildinfo.Info().Version.String(),
	}
	initRequest.Params.Capabilities = mcp.ClientCapabilities{
		Experimental: make(map[string]interface{}),
		Roots:        nil,
		Sampling:     nil,
	}

	_, err = mcpClient.Initialize(ctx, initRequest)

	if err != nil {
		return err
	}

	err = addToolsToServer(ctx, mcpClient, mcpServer)
	if err != nil {
		return err
	}

	_ = addPromptsToServer(ctx, mcpClient, mcpServer)
	_ = addResourcesToServer(ctx, mcpClient, mcpServer)
	_ = addResourceTemplatesToServer(ctx, mcpClient, mcpServer)
	_ = addNotificationsToServer(ctx, mcpClient, mcpServer)

	return nil
}

func startPingTask(ctx context.Context, mcpClient *client.Client) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
PingLoop:
	for {
		select {
		case <-ctx.Done():
			break PingLoop
		case <-ticker.C:
			_ = mcpClient.Ping(ctx)
		}
	}
}

func addToolsToServer(ctx context.Context, mcpClient *client.Client, mcpServer *server.MCPServer) error {
	toolsRequest := mcp.ListToolsRequest{}

	for {
		tools, err := mcpClient.ListTools(ctx, toolsRequest)

		if err != nil {
			return err
		}

		if len(tools.Tools) == 0 {
			break
		}

		for _, tool := range tools.Tools {
			mcpServer.AddTool(tool, mcpClient.CallTool)
		}

		if tools.NextCursor == "" {
			break
		}

		toolsRequest.Params.Cursor = tools.NextCursor
	}

	return nil
}

func addPromptsToServer(ctx context.Context, mcpClient *client.Client, mcpServer *server.MCPServer) error {
	promptsRequest := mcp.ListPromptsRequest{}
	for {
		prompts, err := mcpClient.ListPrompts(ctx, promptsRequest)

		if err != nil {
			return err
		}

		if len(prompts.Prompts) == 0 {
			break
		}

		for _, prompt := range prompts.Prompts {
			mcpServer.AddPrompt(prompt, mcpClient.GetPrompt)
		}

		if prompts.NextCursor == "" {
			break
		}

		promptsRequest.Params.Cursor = prompts.NextCursor
	}
	return nil
}

func addResourcesToServer(ctx context.Context, mcpClient *client.Client, mcpServer *server.MCPServer) error {
	resourcesRequest := mcp.ListResourcesRequest{}

	for {
		resources, err := mcpClient.ListResources(ctx, resourcesRequest)

		if err != nil {
			return err
		}

		if len(resources.Resources) == 0 {
			break
		}

		for _, resource := range resources.Resources {
			mcpServer.AddResource(resource, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				readResource, e := mcpClient.ReadResource(ctx, request)
				if e != nil {
					return nil, e
				}
				return readResource.Contents, nil
			})
		}

		if resources.NextCursor == "" {
			break
		}

		resourcesRequest.Params.Cursor = resources.NextCursor
	}

	return nil
}

func addResourceTemplatesToServer(ctx context.Context, mcpClient *client.Client, mcpServer *server.MCPServer) error {
	resourceTemplatesRequest := mcp.ListResourceTemplatesRequest{}

	for {
		resourceTemplates, err := mcpClient.ListResourceTemplates(ctx, resourceTemplatesRequest)

		if err != nil {
			return err
		}

		if len(resourceTemplates.ResourceTemplates) == 0 {
			break
		}

		for _, resourceTemplate := range resourceTemplates.ResourceTemplates {
			mcpServer.AddResourceTemplate(resourceTemplate, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				readResource, e := mcpClient.ReadResource(ctx, request)
				if e != nil {
					return nil, e
				}
				return readResource.Contents, nil
			})
		}

		if resourceTemplates.NextCursor == "" {
			break
		}

		resourceTemplatesRequest.Params.Cursor = resourceTemplates.NextCursor
	}

	return nil
}

func addNotificationsToServer(ctx context.Context, mcpClient *client.Client, mcpServer *server.MCPServer) error {
	mcpClient.OnNotification(func(notification mcp.JSONRPCNotification) {
		mcpServer.SendNotificationToAllClients(notification.Notification.Method, notification.Notification.Params.AdditionalFields)
	})

	return nil
}
