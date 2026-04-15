package shepherd

import "context"

type botIDContextKey struct{}

// WithBotID stores bot id into context for downstream security event attribution.
func WithBotID(ctx context.Context, botID string) context.Context {
	if ctx == nil || botID == "" {
		return ctx
	}
	return context.WithValue(ctx, botIDContextKey{}, botID)
}

func botIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, ok := ctx.Value(botIDContextKey{}).(string)
	if !ok {
		return ""
	}
	return v
}
