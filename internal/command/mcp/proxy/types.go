package mcpProxy

type ProxyInfo struct {
	Url         string
	BearerToken string
	User        string
	Password    string
	Instance    string
	Mode        string // "passthru" or "sse" or "stream"
	Timeout     int    // Timeout in seconds for the request
	Ping        bool
}
