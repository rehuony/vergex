package api

import (
	"github.com/gin-gonic/gin"
)

// route registers an HTTP route with a one-line description.
func (s *Server) route(g *gin.RouterGroup, method, path, description string, h gin.HandlerFunc) {
	s.routeWithSchema(g, method, path, description, "", h)
}

// routeWithSchema registers an HTTP route. description and schema document
// the route's purpose, parameters and response next to its registration.
func (s *Server) routeWithSchema(g *gin.RouterGroup, method, path, description, schema string, h gin.HandlerFunc) {
	_, _ = description, schema
	switch method {
	case "GET":
		g.GET(path, h)
	case "POST":
		g.POST(path, h)
	case "PUT":
		g.PUT(path, h)
	case "DELETE":
		g.DELETE(path, h)
	}
}
