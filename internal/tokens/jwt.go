package tokens

import (
	"errors"
	"fmt"
	"github.com/golang-jwt/jwt/v4"
)

type NamespaceDetails struct {
	Namespace       string
	EnvironmentId   string
	ProjectName     string
	EnvironmentName string
}

func GenerateTokenForNamespace(secret string, details NamespaceDetails) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"Namespace":       details.Namespace,
		"EnvironmentId":   details.EnvironmentId,
		"ProjectName":     details.ProjectName,
		"EnvironmentName": details.EnvironmentName,
	})
	// Sign and get the complete encoded token as a string using the secret
	tokenString, err := token.SignedString([]byte(secret))
	return tokenString, err
}

func ValidateAndExtractNamespaceDetailsFromToken(key, token string) (NamespaceDetails, error) {

	outToken, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
		// Don't forget to validate the alg is what you expect:
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
		}

		return []byte(key), nil
	})

	ret := NamespaceDetails{}

	if claims, ok := outToken.Claims.(jwt.MapClaims); ok && outToken.Valid {
		if environmentId, ok := claims["EnvironmentId"]; ok {
			//return fmt.Sprintf("%v", EnvironmentId), nil
			ret.EnvironmentId = fmt.Sprintf("%v", environmentId)
		} else {
			return ret, errors.New("Unable to find key 'EnvironmentId' in valid token")
		}

		if namespace, ok := claims["Namespace"]; ok {
			ret.Namespace = fmt.Sprintf("%v", namespace)
		} else {
			return ret, errors.New("Unable to find key 'Namespace' in valid token")
		}

		if environmentName, ok := claims["EnvironmentName"]; ok {
			//return fmt.Sprintf("%v", EnvironmentId), nil
			ret.EnvironmentName = fmt.Sprintf("%v", environmentName)
		} else {
			return ret, errors.New("Unable to find key 'EnvironmentName' in valid token")
		}

		if projectName, ok := claims["ProjectName"]; ok {
			//return fmt.Sprintf("%v", EnvironmentId), nil
			ret.ProjectName = fmt.Sprintf("%v", projectName)
		} else {
			return ret, errors.New("Unable to find key 'ProjectName' in valid token")
		}
	} else {
		return ret, err
	}
	return ret, nil
}
