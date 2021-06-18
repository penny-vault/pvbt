package middleware

import (
	"github.com/gofiber/fiber/v2"
	jwtware "github.com/jdfergason/jwt/v2"
	"github.com/lestrrat-go/jwx/jwk"
)

// JWTAuth instantiate JWT auth middleware
func JWTAuth(jwks *jwk.AutoRefresh, jwksUrl string) fiber.Handler {
	return jwtware.New(jwtware.Config{
		Jwks:         jwks,
		JwksUrl:      jwksUrl,
		ErrorHandler: jwtError,
	})
}

func jwtError(c *fiber.Ctx, err error) error {
	if err.Error() == "Missing or malformed JWT" {
		return c.Status(fiber.StatusBadRequest).
			JSON(fiber.Map{"status": "error", "message": "Missing or malformed JWT", "data": nil})
	}
	return c.Status(fiber.StatusUnauthorized).
		JSON(fiber.Map{"status": "error", "message": "Invalid or expired JWT", "data": nil})
}
