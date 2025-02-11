//go:generate mockgen -package mock -destination mock/jwt.go --source=jwt.go
package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	ginmiddleware "github.com/oapi-codegen/gin-middleware"
)

const jwtContextKey = "nhost/auth/jwt"

type JWTSecret struct {
	Key             string `json:"key"`
	Type            string `json:"type"`
	Issuer          string `json:"issuer"`
	ClaimsNamespace string `json:"claims_namespace"`
}

func decodeJWTSecret(jwtSecretb []byte) (JWTSecret, error) {
	var jwtSecret JWTSecret
	if err := json.Unmarshal(jwtSecretb, &jwtSecret); err != nil {
		return JWTSecret{}, fmt.Errorf("error unmarshalling jwt secret: %w", err)
	}

	if jwtSecret.Issuer == "" {
		jwtSecret.Issuer = "hasura-auth"
	}

	if jwtSecret.ClaimsNamespace == "" {
		jwtSecret.ClaimsNamespace = "https://hasura.io/jwt/claims"
	}

	return jwtSecret, nil
}

type CustomClaimer interface {
	GetClaims(ctx context.Context, userID string) (map[string]any, error)
}

type JWTGetter struct {
	claimsNamespace      string
	issuer               string
	signingKey           []byte
	method               jwt.SigningMethod
	customClaimer        CustomClaimer
	accessTokenExpiresIn time.Duration
}

func NewJWTGetter(
	jwtSecretb []byte,
	accessTokenExpiresIn time.Duration,
	customClaimer CustomClaimer,
) (*JWTGetter, error) {
	jwtSecret, err := decodeJWTSecret(jwtSecretb)
	if err != nil {
		return nil, err
	}

	method := jwt.GetSigningMethod(jwtSecret.Type)

	return &JWTGetter{
		claimsNamespace:      jwtSecret.ClaimsNamespace,
		issuer:               jwtSecret.Issuer,
		signingKey:           []byte(jwtSecret.Key),
		method:               method,
		customClaimer:        customClaimer,
		accessTokenExpiresIn: accessTokenExpiresIn,
	}, nil
}

func pgEncode(v any) (string, error) {
	if v == nil {
		return "null", nil
	}

	if reflect.TypeOf(v).Kind() == reflect.Slice {
		b, err := json.Marshal(v)
		if err != nil {
			return "", fmt.Errorf("error marshalling: %w", err)
		}
		b[0] = '{'
		b[len(b)-1] = '}'
		return string(b), nil
	}

	switch v := v.(type) {
	case string:
		return v, nil
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return "", fmt.Errorf("error marshalling: %w", err)
		}
		return string(b), nil
	}
}

func (j *JWTGetter) GetToken(
	ctx context.Context,
	userID uuid.UUID,
	allowedRoles []string,
	defaultRole string,
	logger *slog.Logger,
) (string, int64, error) {
	now := time.Now()
	iat := now.Unix()
	exp := now.Add(j.accessTokenExpiresIn).Unix()

	var customClaims map[string]any
	var err error
	if j.customClaimer != nil {
		customClaims, err = j.customClaimer.GetClaims(ctx, userID.String())
		if err != nil {
			logger.Error("error getting custom claims", slog.String("error", err.Error()))
			customClaims = map[string]any{}
		}
	}

	c := map[string]any{
		"x-hasura-allowed-roles":    allowedRoles,
		"x-hasura-default-role":     defaultRole,
		"x-hasura-user-id":          userID.String(),
		"x-hasura-user-isAnonymous": "false",
	}

	for k, v := range customClaims {
		value, err := pgEncode(v)
		if err != nil {
			return "", 0, fmt.Errorf("error encoding custom claim: %w", err)
		}

		k = strings.ToLower("x-hasura-" + k)
		if _, ok := c[k]; ok {
			// we do not allow custom claims to overwrite the default claims
			continue
		}
		c[k] = value
	}

	// Create the Claims
	claims := &jwt.MapClaims{
		"sub":             userID.String(),
		"iss":             j.issuer,
		"iat":             iat,
		"exp":             exp,
		j.claimsNamespace: c,
	}
	token := jwt.NewWithClaims(j.method, claims)
	ss, err := token.SignedString(j.signingKey)
	if err != nil {
		return "", 0, fmt.Errorf("error signing token: %w", err)
	}

	return ss, exp, nil
}

func (j *JWTGetter) Validate(accessToken string) (*jwt.Token, error) {
	jwtToken, err := jwt.Parse(
		accessToken,
		func(token *jwt.Token) (interface{}, error) {
			return j.signingKey, nil
		},
		jwt.WithValidMethods([]string{j.method.Alg()}),
		jwt.WithIssuer(j.issuer),
		jwt.WithIssuedAt(),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, fmt.Errorf("error parsing token: %w", err)
	}
	return jwtToken, nil
}

func (j *JWTGetter) FromContext(ctx context.Context) (*jwt.Token, bool) {
	token, ok := ctx.Value(jwtContextKey).(*jwt.Token)
	return token, ok
}

func (j *JWTGetter) ToContext(ctx context.Context, jwtToken *jwt.Token) context.Context {
	return context.WithValue(ctx, jwtContextKey, jwtToken) //nolint:revive,staticcheck
}

func (j *JWTGetter) MiddlewareFunc(
	ctx context.Context, input *openapi3filter.AuthenticationInput,
) error {
	authHeader := input.RequestValidationInput.Request.Header.Get("Authorization")
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return fmt.Errorf("invalid authorization header") //nolint:goerr113
	}

	jwtToken, err := j.Validate(parts[1])
	if err != nil {
		return fmt.Errorf("error validating token: %w", err)
	}

	if !jwtToken.Valid {
		return fmt.Errorf("invalid token") //nolint:goerr113
	}

	c := ginmiddleware.GetGinContext(ctx)
	c.Set(jwtContextKey, jwtToken)

	return nil
}

func (j *JWTGetter) GetCustomClaim(token *jwt.Token, customClaim string) string {
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return ""
	}
	customClaims, ok := claims[j.claimsNamespace].(map[string]any)
	if !ok {
		return ""
	}

	v, ok := customClaims[customClaim].(string)
	if !ok {
		return ""
	}

	return v
}
