package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

func SetCanvasRouter(router *gin.Engine) {
	canvas := router.Group("/api/canvas", middleware.UserAuth(), middleware.CanvasSessionOnly())
	relay := canvas.Group("/relay")
	for _, protocol := range pluginruntime.HostProtocols() {
		for _, operation := range protocol.Operations {
			handlers, err := taskPluginProtocolHandlers(protocol.Name, operation.Name, middleware.CanvasRelayAuth())
			if err != nil {
				panic(err)
			}
			for _, method := range operation.Methods {
				relay.Handle(method, operation.Path, handlers...)
			}
		}
	}
	relay.POST("/v1/chat/completions", middleware.RouteTag("relay"), middleware.CanvasRelayAuth(), middleware.SystemPerformanceCheck(), middleware.ModelRequestRateLimit(), middleware.Distribute(), func(c *gin.Context) { controller.Relay(c, types.RelayFormatOpenAI) })
	relay.POST("/v1/audio/speech", middleware.RouteTag("relay"), middleware.CanvasRelayAuth(), middleware.SystemPerformanceCheck(), middleware.ModelRequestRateLimit(), middleware.Distribute(), func(c *gin.Context) { controller.Relay(c, types.RelayFormatOpenAIAudio) })
	relay.POST("/v1beta/models/*path", middleware.RouteTag("relay"), middleware.CanvasRelayAuth(), middleware.SystemPerformanceCheck(), middleware.ModelRequestRateLimit(), middleware.Distribute(), func(c *gin.Context) { controller.Relay(c, types.RelayFormatGemini) })
	relay.GET("/v1/tasks/:key", middleware.RouteTag("relay"), middleware.CanvasRelayAuth(), controller.GetTask)
	relay.GET("/v1/tasks/:key/artifacts", middleware.RouteTag("relay"), middleware.CanvasRelayAuth(), controller.GetTaskArtifacts)
	relay.GET("/v1/tasks/:key/artifacts/:artifact_key/content", middleware.RouteTag("relay"), middleware.CanvasRelayAuth(), controller.TaskArtifactContent)
}
