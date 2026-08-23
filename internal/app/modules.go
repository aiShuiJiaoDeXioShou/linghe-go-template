package app

import (
	"time"

	"go-template/internal/auth"
	"go-template/internal/data"
	"go-template/internal/health"
	adminusermodule "go-template/internal/modules/adminuser"
	configmodule "go-template/internal/modules/config"
	usermodule "go-template/internal/modules/user"

	"github.com/gofiber/fiber/v3"
)

// registerModules 集中装配认证 业务模块和系统探针
func registerModules(
	router fiber.Router,
	resources *data.Data,
	realms *auth.Realms,
	readinessTimeout time.Duration,
) {
	passwords := auth.NewPasswordHasher()

	// 装配业务用户和 App 登录域
	userService := usermodule.NewService(
		usermodule.NewRepository(resources),
		passwords,
		auth.NewSessionIssuer(realms.App),
	)
	usermodule.RegisterHandlers(router, userService, realms.App)

	// 装配管理员用户和 Admin 登录域
	adminUserService := adminusermodule.NewService(
		adminusermodule.NewRepository(resources),
		passwords,
		auth.NewSessionIssuer(realms.Admin),
	)
	adminusermodule.RegisterHandlers(router, adminUserService, realms.Admin)

	// 装配系统配置双端入口
	configService := configmodule.NewService(configmodule.NewRepository(resources))
	configmodule.RegisterHandlers(router, configService, realms.Admin)

	// 注册不经过业务分层的系统探针
	health.RegisterHandlers(
		router,
		resources.Ping,
		resources.PingDatabase,
		readinessTimeout,
	)

	// 注册双登录域的公共会话接口
	auth.RegisterHandlers(router, realms)
}
