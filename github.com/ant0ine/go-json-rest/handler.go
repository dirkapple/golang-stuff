// A quick and easy way to setup a RESTful JSON API
//
// Go-Json-Rest is a thin layer on top of net/http that helps building RESTful JSON APIs easily.
// It provides fast URL routing using a Trie based implementation, and helpers to deal
// with JSON requests and responses. It is not a high-level REST framework that transparently maps
// HTTP requests to procedure calls, on the opposite, you constantly have access to the underlying
// net/http objects.
//
// Example:
//
//      package main
//
//      import (
//              "github.com/ant0ine/go-json-rest"
//              "net/http"
//      )
//
//      type User struct {
//              Id   string
//              Name string
//      }
//
//      func GetUser(w *rest.ResponseWriter, req *rest.Request) {
//              user := User{
//                      Id:   req.PathParam("id"),
//                      Name: "Antoine",
//              }
//              w.WriteJson(&user)
//      }
//
//      func main() {
//              handler := rest.ResourceHandler{}
//              handler.SetRoutes(
//                      rest.Route{"GET", "/users/:id", GetUser},
//              )
//              http.ListenAndServe(":8080", &handler)
//      }
//
//
// Note about the URL routing: Instead of using the usual
// "evaluate all the routes and return the first regexp that matches" strategy,
// it uses a Trie data structure to perform the routing. This is more efficient,
// and scales better for a large number of routes.
// It supports the :param and *splat placeholders in the route strings.
//
package rest

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"reflect"
	"runtime/debug"
	"strings"
	"time"
)

// Implement the http.Handler interface and act as a router for the defined Routes.
// The defaults are intended to be developemnt friendly, for production you may want
// to turn on gzip and disable the JSON indentation.
type ResourceHandler struct {
	internalRouter *router
	statusService  *statusService

	// If true, and if the client accepts the Gzip encoding, the response payloads
	// will be compressed using gzip, and the corresponding response header will set.
	EnableGzip bool

	// If true, the JSON payload will be written in one line with no space.
	DisableJsonIndent bool

	// If true, the status service will be enabled. Various stats and status will
	// then be available at GET /.status in a JSON format.
	EnableStatusService bool

	// If true, when a "panic" happens, the error string and the stack trace will be
	// printed in the 500 response body.
	EnableResponseStackTrace bool

	// If true, the record that is logged for each response will be printed as JSON
	// in the log. Convenient for log parsing.
	EnableLogAsJson bool

	// Custom logger, defaults to log.New(os.Stderr, "", log.LstdFlags)
	Logger *log.Logger
}

// Used with SetRoutes.
type Route struct {

	// Any http method. It will be used as uppercase to avoid common mistakes.
	HttpMethod string

	// A string like "/resource/:id.json".
	// Placeholders supported are:
	// :param that matches any char to the first '/' or '.'
	// *splat that matches everything to the end of the string
	// (placeholder names should be unique per PathExp)
	PathExp string

	// Code that will be executed when this route is taken.
	Func func(*ResponseWriter, *Request)
}

// Create a Route that points to an object method. It can be convenient to point to an object method instead
// of a function, this helper makes it easy by passing the object instance and the method name as parameters.
func RouteObjectMethod(httpMethod string, pathExp string, objectInstance interface{}, objectMethod string) Route {

	value := reflect.ValueOf(objectInstance)
	funcValue := value.MethodByName(objectMethod)
	if funcValue.IsValid() == false {
		panic(fmt.Sprintf(
			"Cannot find the object method %s on %s",
			objectMethod,
			value,
		))
	}
	routeFunc := func(w *ResponseWriter, r *Request) {
		funcValue.Call([]reflect.Value{
			reflect.ValueOf(w),
			reflect.ValueOf(r),
		})
	}

	return Route{
		HttpMethod: httpMethod,
		PathExp:    pathExp,
		Func:       routeFunc,
	}
}

// Define the Routes. The order the Routes matters,
// if a request matches multiple Routes, the first one will be used.
func (self *ResourceHandler) SetRoutes(routes ...Route) error {

	self.internalRouter = &router{
		routes: routes,
	}

	// add the status route as the last route.
	if self.EnableStatusService == true {
		self.statusService = newStatusService()
		self.internalRouter.routes = append(
			self.internalRouter.routes,
			Route{
				HttpMethod: "GET",
				PathExp:    "/.status",
				Func: func(writer *ResponseWriter, request *Request) {
					self.statusService.getStatus(writer, request)
				},
			},
		)
	}

	// start the router
	err := self.internalRouter.start()
	if err != nil {
		return err
	}

	return nil
}

type responseLogRecord struct {
	StatusCode   int
	ResponseTime *time.Duration
	HttpMethod   string
	RequestURI   string
}

func (self *ResourceHandler) logResponseRecord(record *responseLogRecord) {
	if self.EnableLogAsJson {
		b, err := json.Marshal(record)
		if err != nil {
			panic(err)
		}
		self.Logger.Printf("%s", b)
	} else {
		self.Logger.Printf("%d %v %s %s",
			record.StatusCode,
			record.ResponseTime,
			record.HttpMethod,
			record.RequestURI,
		)
	}
}

func (self *ResourceHandler) logResponse(statusCode int, start *time.Time, request *http.Request) {

	now := time.Now()
	duration := now.Sub(*start)

	if self.statusService != nil {
		self.statusService.update(statusCode, &duration)
	}

	self.logResponseRecord(&responseLogRecord{
		statusCode,
		&duration,
		request.Method,
		request.URL.RequestURI(),
	})
}

// This makes ResourceHandler implement the http.Handler interface.
// You probably don't want to use it directly.
func (self *ResourceHandler) ServeHTTP(origWriter http.ResponseWriter, origRequest *http.Request) {

	start := time.Now()

	// set a default Logger
	if self.Logger == nil {
		self.Logger = log.New(os.Stderr, "", log.LstdFlags)
	}

	// catch user code's panic, and convert to http response
	// (this does not use the JSON error response on purpose)
	defer func() {
		if reco := recover(); reco != nil {
			trace := debug.Stack()

			// log the trace
			self.Logger.Printf("%s\n%s", reco, trace)

			// write error response
			message := "Internal Server Error"
			if self.EnableResponseStackTrace {
				message = fmt.Sprintf("%s\n\n%s", reco, trace)
			}
			http.Error(origWriter, message, http.StatusInternalServerError)

			// log response
			self.logResponse(
				http.StatusInternalServerError,
				&start,
				origRequest,
			)
		}
	}()

	request := Request{
		origRequest,
		nil,
	}

	// determine if gzip is needed
	isGzipped := self.EnableGzip == true &&
		strings.Contains(origRequest.Header.Get("Accept-Encoding"), "gzip")

	isIndented := !self.DisableJsonIndent

	writer := ResponseWriter{
		origWriter,
		isGzipped,
		isIndented,
		0,
		false,
	}

	// find the route
	route, params, pathMatched := self.internalRouter.findRouteFromURL(origRequest.Method, origRequest.URL)
	if route == nil {
		if pathMatched {
			// no route found, but path was matched: 405 Method Not Allowed
			Error(&writer, "Method not allowed", http.StatusMethodNotAllowed)

			// log response
			self.logResponse(
				http.StatusMethodNotAllowed,
				&start,
				origRequest,
			)
			return
		} else {
			// no route found, the path was not matched: 404 Not Found
			NotFound(&writer, &request)

			// log response
			self.logResponse(
				http.StatusNotFound,
				&start,
				origRequest,
			)
			return
		}
	}

	// a route was found, set the PathParams
	request.PathParams = params

	// run the user code
	handler := route.Func
	handler(&writer, &request)

	// log response
	self.logResponse(
		writer.statusCode,
		&start,
		origRequest,
	)
}
