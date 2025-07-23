package gitlab

import (
	"context"
	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
	"go.opentelemetry.io/otel"
)

func (c *Client) ListRunners(ctx context.Context) (runners schemas.Runners, err error) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "gitlab:ListRunners")
	defer span.End()

	// TODO: Add tracing attributes for Runners
	//span.SetAttributes(attribute.String("runners"))
	
	return
}
