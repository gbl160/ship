// Copyright 2019 xgfone
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/xgfone/ship/v2"
)

// CORSConfig is used to configure the CORS middleware.
type CORSConfig struct {
	// AllowOrigin defines a list of origins that may access the resource.
	//
	// Optional. Default: []string{"*"}.
	AllowOrigins []string

	// AllowHeaders indicates a list of request headers used in response to
	// a preflight request to indicate which HTTP headers can be used when
	// making the actual request. This is in response to a preflight request.
	//
	// Optional. Default: []string{}.
	AllowHeaders []string

	// AllowMethods indicates methods allowed when accessing the resource.
	// This is used in response to a preflight request.
	//
	// Optional. Default: []string{"HEAD", "GET", "POST", "PUT", "PATHC", "DELETE"}.
	AllowMethods []string

	// ExposeHeaders indicates a server whitelist headers that browsers are
	// allowed to access. This is in response to a preflight request.
	//
	// Optional. Default: []string{}.
	ExposeHeaders []string

	// AllowCredentials indicates whether or not the response to the request
	// can be exposed when the credentials flag is true. When used as part of
	// a response to a preflight request, this indicates whether or not the
	// actual request can be made using credentials.
	//
	// Optional. Default: false.
	AllowCredentials bool

	// MaxAge indicates how long (in seconds) the results of a preflight request
	// can be cached.
	//
	// Optional. Default: 0.
	MaxAge int
}

// CORS returns a CORS middleware.
//
// If the config is missing, it will use:
//
//   conf := CORSConfig{
//       AllowOrigins: []string{"*"},
//       AllowMethods: []string{"HEAD", "GET", "POST", "PUT", "PATHC", "DELETE"},
//   }
//
func CORS(config ...CORSConfig) Middleware {
	var conf CORSConfig
	if len(config) > 0 {
		conf = config[0]
	}

	if len(conf.AllowOrigins) == 0 {
		conf.AllowOrigins = []string{"*"}
	}
	if len(conf.AllowMethods) == 0 {
		conf.AllowMethods = []string{http.MethodHead, http.MethodGet,
			http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}
	}

	allowMethods := strings.Join(conf.AllowMethods, ",")
	allowHeaders := strings.Join(conf.AllowHeaders, ",")
	exposeHeaders := strings.Join(conf.ExposeHeaders, ",")
	maxAge := fmt.Sprintf("%d", conf.MaxAge)

	return func(next ship.Handler) ship.Handler {
		return func(ctx *ship.Context) error {
			// Check whether the origin is allowed or not.
			var allowOrigin string
			origin := ctx.GetHeader(ship.HeaderOrigin)
			for _, o := range conf.AllowOrigins {
				if o == "*" {
					if conf.AllowCredentials {
						allowOrigin = origin
					} else {
						allowOrigin = o
					}
				} else if o == origin {
					allowOrigin = o
					break
				}

				if matchSubdomain(origin, o) {
					allowOrigin = origin
					break
				}
			}

			// Simple request
			if ctx.Method() != http.MethodOptions {
				ctx.AddHeader(ship.HeaderVary, ship.HeaderOrigin)
				ctx.SetHeader(ship.HeaderAccessControlAllowOrigin, allowOrigin)
				if conf.AllowCredentials {
					ctx.SetHeader(ship.HeaderAccessControlAllowCredentials, "true")
				}
				if exposeHeaders != "" {
					ctx.SetHeader(ship.HeaderAccessControlExposeHeaders, exposeHeaders)
				}
				return next(ctx)
			}

			// Preflight request
			ctx.AddHeader(ship.HeaderVary, ship.HeaderOrigin)
			ctx.AddHeader(ship.HeaderVary, ship.HeaderAccessControlRequestMethod)
			ctx.AddHeader(ship.HeaderVary, ship.HeaderAccessControlRequestHeaders)
			ctx.SetHeader(ship.HeaderAccessControlAllowOrigin, allowOrigin)
			ctx.SetHeader(ship.HeaderAccessControlAllowMethods, allowMethods)

			if conf.AllowCredentials {
				ctx.SetHeader(ship.HeaderAccessControlAllowCredentials, "true")
			}

			if allowHeaders != "" {
				ctx.SetHeader(ship.HeaderAccessControlAllowHeaders, allowHeaders)
			} else if h := ctx.GetHeader(ship.HeaderAccessControlRequestHeaders); h != "" {
				ctx.SetHeader(ship.HeaderAccessControlAllowHeaders, h)
			}

			if conf.MaxAge > 0 {
				ctx.SetHeader(ship.HeaderAccessControlMaxAge, maxAge)
			}

			return ctx.NoContent(http.StatusNoContent)
		}
	}
}
