package tokens

import (
	"errors"
	"fmt"
	"github.com/golang-jwt/jwt/v4"
)

func GenerateTokenForNamespace(secret, namespaceName string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"namespace": namespaceName,
	})
	// Sign and get the complete encoded token as a string using the secret
	tokenString, err := token.SignedString([]byte(secret))
	return tokenString, err
}

func ValidateAndExtractNamespaceFromToken(key, token string) (string, error) {

	outToken, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
		// Don't forget to validate the alg is what you expect:
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
		}

		return []byte(key), nil
	})

	if claims, ok := outToken.Claims.(jwt.MapClaims); ok && outToken.Valid {
		if namespaceName, ok := claims["namespace"]; ok {
			return fmt.Sprintf("%v", namespaceName), nil
		} else {
			return "", errors.New("Unable to find key 'namespace' in valid token")
		}

	} else {
		return "", err
	}
}
