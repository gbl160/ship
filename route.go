// Copyright 2018 xgfone
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

package ship

import (
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/pprof"
	"os"
	"path"
	"reflect"
	"runtime"
	rpprof "runtime/pprof"
	"strconv"
	"strings"

	"github.com/xgfone/ship/v2/router"
)

// AllMethods represents all HTTP methods.
var AllMethods = []string{
	http.MethodConnect, http.MethodHead, http.MethodOptions, http.MethodTrace,
	http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete,
	http.MethodPatch,
}

// RouteFilter is used to filter the registering route if it returns true.
type RouteFilter func(RouteInfo) bool

// RouteModifier is used to modify the registering route.
type RouteModifier func(RouteInfo) RouteInfo

type kvalues struct {
	Key    string
	Values []string
}

// RouteInfo is used to represent the information of the registered route.
type RouteInfo struct {
	Name    string        `json:"name" xml:"name"`
	Host    string        `json:"host" xml:"host"`
	Path    string        `json:"path" xml:"path"`
	Method  string        `json:"method" xml:"method"`
	Handler Handler       `json:"-" xml:"-"`
	Router  router.Router `json:"-" xml:"-"`
}

type pprofHandler string

func (name pprofHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	p := rpprof.Lookup(string(name))
	if p == nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("X-Go-Pprof", "1")
		w.Header().Del("Content-Disposition")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintln(w, "Unknown profile")
		return
	}
	gc, _ := strconv.Atoi(r.FormValue("gc"))
	if name == "heap" && gc > 0 {
		runtime.GC()
	}
	debug, _ := strconv.Atoi(r.FormValue("debug"))
	if debug != 0 {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	}
	p.WriteTo(w, debug)
}

// HTTPPprofToRouteInfo converts http pprof handler to RouteInfo,
// so that you can register them and get runtime profiling data by HTTP server.
func HTTPPprofToRouteInfo() []RouteInfo {
	return []RouteInfo{
		{
			Name:   "pprof_index",
			Path:   "/debug/pprof/*",
			Method: http.MethodGet,
			Handler: func(ctx *Context) error {
				path := ctx.Path()
				i := strings.Index(path, "/debug/pprof/")
				if _len := i + 13; len(path) > _len {
					pprofHandler(path[_len:]).ServeHTTP(ctx.Response(), ctx.Request())
					return nil
				}
				pprof.Index(ctx.Response(), ctx.Request())
				return nil
			},
		},
		{
			Name:    "pprof_cmdline",
			Path:    "/debug/pprof/cmdline",
			Method:  http.MethodGet,
			Handler: FromHTTPHandlerFunc(pprof.Cmdline),
		},
		{
			Name:    "pprof_profile",
			Path:    "/debug/pprof/profile",
			Method:  http.MethodGet,
			Handler: FromHTTPHandlerFunc(pprof.Profile),
		},
		{
			Name:    "pprof_symbol",
			Path:    "/debug/pprof/symbol",
			Method:  http.MethodGet,
			Handler: FromHTTPHandlerFunc(pprof.Symbol),
		},
		{
			Name:    "pprof_symbol",
			Path:    "/debug/pprof/symbol",
			Method:  http.MethodPost,
			Handler: FromHTTPHandlerFunc(pprof.Symbol),
		},
		{
			Name:    "pprof_trace",
			Path:    "/debug/pprof/trace",
			Method:  http.MethodGet,
			Handler: FromHTTPHandlerFunc(pprof.Trace),
		},
	}
}

// Route represents a route information.
type Route struct {
	ship    *Ship
	group   *RouteGroup
	host    string
	path    string
	name    string
	mdwares []Middleware
	headers []kvalues
}

func newRoute(s *Ship, g *RouteGroup, prefix, host, path string,
	ms ...Middleware) *Route {
	if path == "" {
		panic("the route path must not be empty")
	} else if path[0] != '/' {
		panic(fmt.Errorf("path '%s' must start with '/'", path))
	}

	return &Route{
		ship:    s,
		group:   g,
		host:    host,
		path:    strings.TrimSuffix(prefix, "/") + path,
		mdwares: append([]Middleware{}, ms...),
	}
}

// New clones a new Route based on the current route.
func (r *Route) New() *Route {
	return &Route{
		ship:  r.ship,
		host:  r.host,
		path:  r.path,
		name:  r.name,
		group: r.group,

		mdwares: append([]Middleware{}, r.mdwares...),
		headers: append([]kvalues{}, r.headers...),
	}
}

// Ship returns the ship that the current route is associated with.
func (r *Route) Ship() *Ship { return r.ship }

// Group returns the group that the current route belongs to.
//
// Notice: it will return nil if the route is from ship.Route.
func (r *Route) Group() *RouteGroup { return r.group }

// NoMiddlewares clears all the middlewares and returns itself.
func (r *Route) NoMiddlewares() *Route { r.mdwares = nil; return r }

// Name sets the route name.
func (r *Route) Name(name string) *Route { r.name = name; return r }

// Host sets the host of the route to host.
func (r *Route) Host(host string) *Route { r.host = host; return r }

// Use adds some middlwares for the route.
func (r *Route) Use(middlewares ...Middleware) *Route {
	r.mdwares = append(r.mdwares, middlewares...)
	return r
}

// HasHeader checks whether the request contains the request header.
// If no, the request will be rejected.
//
// If the header value is given, it will be tested to match.
//
// Example
//
//     s := ship.New()
//     // The request must contains the header "Content-Type: application/json".
//     s.R("/path/to").HasHeader("Content-Type", "application/json").POST(handler)
//
func (r *Route) HasHeader(headerK string, headerV ...string) *Route {
	r.headers = append(r.headers, kvalues{http.CanonicalHeaderKey(headerK), headerV})
	return r
}

func (r *Route) buildHeaderMiddleware() Middleware {
	if len(r.headers) == 0 {
		return nil
	}

	return func(next Handler) Handler {
		return func(ctx *Context) error {
			for _, kv := range r.headers {
				value := ctx.GetHeader(kv.Key)
				if len(kv.Values) == 0 {
					if value == "" {
						err := fmt.Errorf("missing the header '%s'", kv.Key)
						return ErrBadRequest.NewError(err)
					}
				} else {
					for _, v := range kv.Values {
						if v == value {
							return next(ctx)
						}
					}
					err := fmt.Errorf("invalid header '%s: %s'", kv.Key, value)
					return ErrBadRequest.NewError(err)
				}
			}
			return next(ctx)
		}
	}
}

func (r *Route) addRoute(name, host, path string, handler Handler,
	methods ...string) *Route {
	if handler == nil {
		panic(errors.New("handler must not be nil"))
	}

	if len(methods) == 0 {
		panic(errors.New("the route requires methods"))
	}

	if len(path) == 0 || path[0] != '/' {
		panic(fmt.Errorf("path '%s' must start with '/'", path))
	}

	if i := strings.Index(path, "//"); i != -1 {
		panic(fmt.Errorf("bad path '%s' contains duplicate // at index:%d", path, i))
	}

	middlewares := r.mdwares
	if m := r.buildHeaderMiddleware(); m != nil {
		middlewares = make([]Middleware, 0, len(r.mdwares)+1)
		middlewares = append(middlewares, r.mdwares...)
		middlewares = append(middlewares, m)
	}

	middlewaresLen := len(middlewares)
	if middlewaresLen > r.ship.MiddlewareMaxNum {
		panic(fmt.Errorf("the number of middlewares '%d' has exceeded the maximum '%d'",
			middlewaresLen, r.ship.MiddlewareMaxNum))
	}

	for i := middlewaresLen - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}

	for _, method := range methods {
		r.ship.addRoute(name, host, path, method, handler)
	}

	return r
}

// Method sets the methods and registers the route.
//
// If methods is nil, it will register all the supported methods for the route.
//
// Notice: The method must be called at last.
func (r *Route) Method(handler Handler, methods ...string) *Route {
	r.addRoute(r.name, r.host, r.path, handler, methods...)
	return r
}

// Any registers all the supported methods , which is short for
// r.Method(handler, AllMethods...)
func (r *Route) Any(handler Handler) *Route {
	return r.Method(handler, AllMethods...)
}

// CONNECT is the short for r.Method(handler, "CONNECT").
func (r *Route) CONNECT(handler Handler) *Route {
	return r.Method(handler, http.MethodConnect)
}

// OPTIONS is the short for r.Method(handler, "OPTIONS").
func (r *Route) OPTIONS(handler Handler) *Route {
	return r.Method(handler, http.MethodOptions)
}

// HEAD is the short for r.Method(handler, "HEAD").
func (r *Route) HEAD(handler Handler) *Route {
	return r.Method(handler, http.MethodHead)
}

// PATCH is the short for r.Method(handler, "PATCH").
func (r *Route) PATCH(handler Handler) *Route {
	return r.Method(handler, http.MethodPatch)
}

// TRACE is the short for r.Method(handler, "TRACE").
func (r *Route) TRACE(handler Handler) *Route {
	return r.Method(handler, http.MethodTrace)
}

// GET is the short for r.Method(handler, "GET").
func (r *Route) GET(handler Handler) *Route {
	return r.Method(handler, http.MethodGet)
}

// PUT is the short for r.Method(handler, "PUT").
func (r *Route) PUT(handler Handler) *Route {
	return r.Method(handler, http.MethodPut)
}

// POST is the short for r.Method(handler, "POST").
func (r *Route) POST(handler Handler) *Route {
	return r.Method(handler, http.MethodPost)
}

// DELETE is the short for r.Method(handler, "DELETE").
func (r *Route) DELETE(handler Handler) *Route {
	return r.Method(handler, http.MethodDelete)
}

// Redirect is used to redirect the path to toURL.
//
// method is GET by default.
func (r *Route) Redirect(code int, toURL string, method ...string) *Route {
	rmethod := http.MethodGet
	if len(method) > 0 && method[0] != "" {
		rmethod = method[0]
	}

	return r.Method(func(ctx *Context) error {
		return ctx.Redirect(code, toURL)
	}, rmethod)
}

// Map registers a group of methods with handlers, which is equal to
//
//     for method, handler := range method2handlers {
//         r.Method(handler, method)
//     }
func (r *Route) Map(method2handlers map[string]Handler) *Route {
	for method, handler := range method2handlers {
		r.Method(handler, method)
	}
	return r
}

// MapType registers the methods of a type as the routes.
//
// By default, mapping is Ship.Config.DefaultMethodMapping if not given.
//
// Example
//
//    type TestType struct{}
//    func (t TestType) Create(ctx *ship.Context) error { return nil }
//    func (t TestType) Delete(ctx *ship.Context) error { return nil }
//    func (t TestType) Update(ctx *ship.Context) error { return nil }
//    func (t TestType) Get(ctx *ship.Context) error    { return nil }
//    func (t TestType) Has(ctx *ship.Context) error    { return nil }
//    func (t TestType) NotHandler()                   {}
//
//    router := ship.New()
//    router.Route("/path/to").MapType(TestType{})
//
// It's equal to the operation as follow:
//
//    router.Route("/v1/testtype/get").Name("testtype_get").GET(ts.Get)
//    router.Route("/v1/testtype/update").Name("testtype_update").PUT(ts.Update)
//    router.Route("/v1/testtype/create").Name("testtype_create").POST(ts.Create)
//    router.Route("/v1/testtype/delete").Name("testtype_delete").DELETE(ts.Delete)
//
// If you don't like the default mapping policy, you can give the customized
// mapping by the last argument, the key of which is the name of the method
// of the type, and the value of that is the request method, such as GET, POST,
// etc. Notice that the method type must be compatible with
//
//    func (*Context) error
//
// Notice: the name of type and method will be converted to the lower.
func (r *Route) MapType(tv interface{}) *Route {
	if tv == nil {
		panic(errors.New("the type value must no be nil"))
	}

	value := reflect.ValueOf(tv)
	methodMaps := r.ship.MethodMapping
	if methodMaps == nil {
		methodMaps = DefaultMethodMapping
	}

	var err error
	errType := reflect.TypeOf(&err).Elem()
	prefix := r.path
	if prefix == "/" {
		prefix = ""
	}

	_type := value.Type()
	typeName := strings.ToLower(_type.Name())
	for i := _type.NumMethod() - 1; i >= 0; i-- {
		method := _type.Method(i)
		mtype := method.Type

		// func (s StructType) Handler(ctx *Context) error
		if mtype.NumIn() != 2 || mtype.NumOut() != 1 {
			continue
		}
		if _, ok := reflect.New(mtype.In(1)).Interface().(*Context); !ok {
			continue
		}
		if !mtype.Out(0).Implements(errType) {
			continue
		}

		// r.addRoute(r.name, r.path, handler, methods...)
		if reqMethod := methodMaps[method.Name]; reqMethod != "" {
			methodName := strings.ToLower(method.Name)
			path := fmt.Sprintf("%s/%s/%s", prefix, typeName, methodName)

			name := fmt.Sprintf("%s_%s", typeName, methodName)
			r.addRoute(name, r.host, path, func(ctx *Context) error {
				vs := method.Func.Call([]reflect.Value{value, reflect.ValueOf(ctx)})
				return vs[0].Interface().(error)
			}, reqMethod)
		}
	}

	return r
}

func (r *Route) serveFileMetadata(ctx *Context, filename string) error {
	f, err := os.Open(filename)
	if err != nil {
		return NewHTTPError(http.StatusInternalServerError).NewError(err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return NewHTTPError(http.StatusInternalServerError).NewError(err)
	} else if fi.IsDir() {
		return ctx.NotFoundHandler()(ctx)
	}

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return NewHTTPError(http.StatusInternalServerError).NewError(err)
	}

	ctx.SetHeader(HeaderEtag, fmt.Sprintf("%x", h.Sum(nil)))
	ctx.SetHeader(HeaderContentLength, fmt.Sprintf("%d", fi.Size()))
	return ctx.NoContent(http.StatusOK)
}

// StaticFile registers a route for a static file, which supports the HEAD method
// to get the its length and the GET method to download it.
func (r *Route) StaticFile(filePath string) *Route {
	if strings.Contains(r.path, ":") || strings.Contains(r.path, "*") {
		panic(errors.New("URL parameters cannot be used when serving a static file"))
	}

	r.addRoute("", r.host, r.path, func(ctx *Context) error {
		return ctx.File(filePath)
	}, http.MethodGet)
	r.addRoute("", r.host, r.path, func(ctx *Context) error {
		return r.serveFileMetadata(ctx, filePath)
	}, http.MethodHead)
	return r
}

// StaticFS registers a route to serve a static filesystem.
func (r *Route) StaticFS(fs http.FileSystem) *Route {
	if strings.Contains(r.path, ":") || strings.Contains(r.path, "*") {
		panic(errors.New("URL parameters cannot be used when serving a static file"))
	}

	fileServer := http.StripPrefix(r.path, http.FileServer(fs))
	rpath := path.Join(r.path, "/*")

	r.addRoute("", r.host, rpath, func(ctx *Context) error {
		if _, err := fs.Open(ctx.URLParam("*")); err != nil {
			return ctx.NotFoundHandler()(ctx)
		}
		fileServer.ServeHTTP(ctx.Response(), ctx.Request())
		return nil
	}, http.MethodHead, http.MethodGet)

	return r
}

// Static is the same as StaticFS, but listing the files for a directory.
func (r *Route) Static(dirpath string) *Route {
	return r.StaticFS(newOnlyFileFS(dirpath))
}

func newOnlyFileFS(root string) http.FileSystem {
	return onlyFileFS{fs: http.Dir(root)}
}

type onlyFileFS struct {
	fs http.FileSystem
}

func (fs onlyFileFS) Open(name string) (http.File, error) {
	f, err := fs.fs.Open(name)
	if err != nil {
		return nil, err
	}
	return notDirFile{f}, nil
}

type notDirFile struct {
	http.File
}

func (f notDirFile) Readdir(count int) ([]os.FileInfo, error) {
	return nil, nil
}
