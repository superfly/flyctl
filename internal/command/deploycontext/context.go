package deploycontext

// ContextKey is a custom type for deploy-related context keys to avoid collisions
type ContextKey string

// IsFirstLaunchKey is the context key for marking a deployment as the first launch
const IsFirstLaunchKey ContextKey = "isFirstLaunch"
