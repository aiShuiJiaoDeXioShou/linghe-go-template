package httpserver

import (
	_ "go-template/docs/swagger"

	swaggomiddleware "github.com/gofiber/contrib/v3/swaggo"
)

// RegisterAPIDocs 注册 Swagger 文档和调试页面
func (s *Server) RegisterAPIDocs() {
	s.app.Get("/docs/*", swaggomiddleware.New(swaggomiddleware.Config{
		Title:                "go-template API",
		URL:                  "doc.json",
		DeepLinking:          true,
		PersistAuthorization: true,
	}))
}
