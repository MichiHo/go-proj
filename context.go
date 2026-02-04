package proj

// #include <stdlib.h>
// #include "go-proj.h"
import "C"

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"
)

type LogLevel C.PJ_LOG_LEVEL

const (
	LogLevelNone  LogLevel = C.PJ_LOG_NONE
	LogLevelError LogLevel = C.PJ_LOG_ERROR
	LogLevelDebug LogLevel = C.PJ_LOG_DEBUG
	LogLevelTrace LogLevel = C.PJ_LOG_TRACE
	LogLevelTell  LogLevel = C.PJ_LOG_TELL
)

var defaultContext = &Context{}

func init() {
	C.proj_log_level(nil, C.PJ_LOG_NONE)
}

// A Context is a context.
type Context struct {
	mutex     sync.Mutex
	pjContext *C.PJ_CONTEXT
}

// NewContext returns a new Context.
func NewContext() *Context {
	pjContext := C.proj_context_create()
	C.proj_log_level(pjContext, C.PJ_LOG_NONE)
	c := &Context{
		pjContext: pjContext,
	}
	runtime.SetFinalizer(c, (*Context).Destroy)
	return c
}

// Destroy frees all resources associated with c.
func (c *Context) Destroy() {
	c.Lock()
	defer c.Unlock()
	if c.pjContext != nil {
		C.proj_context_destroy(c.pjContext)
		c.pjContext = nil
	}
}

// SetLogLevel sets the log level.
func (c *Context) SetLogLevel(logLevel LogLevel) {
	c.Lock()
	defer c.Unlock()
	C.proj_log_level(c.pjContext, C.PJ_LOG_LEVEL(logLevel))
}

// SetSearchPaths sets the paths PROJ should be exploring to find the PROJ Data files.
func (c *Context) SetSearchPaths(paths []string) {
	c.Lock()
	defer c.Unlock()
	cPaths := make([]*C.char, len(paths))
	var pathPtr unsafe.Pointer
	for i, path := range paths {
		cPaths[i] = C.CString(path)
		defer C.free(unsafe.Pointer(cPaths[i]))
	}
	if len(paths) > 0 {
		pathPtr = unsafe.Pointer(&cPaths[0])
	}
	C.proj_context_set_search_paths(c.pjContext, C.int(len(cPaths)), (**C.char)(pathPtr))
}

func (c *Context) Lock() {
	c.mutex.Lock()
}

// NewCRSToCRS returns a new PJ from sourceCRS to targetCRS and optional area.
func (c *Context) NewCRSToCRS(sourceCRS, targetCRS string, area *Area) (*PJ, error) {
	c.Lock()
	defer c.Unlock()

	cSourceCRS := C.CString(sourceCRS)
	defer C.free(unsafe.Pointer(cSourceCRS))

	cTargetCRS := C.CString(targetCRS)
	defer C.free(unsafe.Pointer(cTargetCRS))

	var cArea *C.PJ_AREA
	if area != nil {
		cArea = area.pjArea
	}

	return c.newPJ(C.proj_create_crs_to_crs(c.pjContext, cSourceCRS, cTargetCRS, cArea))
}

// NewCRSToCRSFromPJ returns a new PJ from two CRSs.
func (c *Context) NewCRSToCRSFromPJ(sourcePJ, targetPJ *PJ, area *Area, options string) (*PJ, error) {
	c.Lock()
	defer c.Unlock()

	if sourcePJ.context != c {
		sourcePJ.context.Lock()
		defer sourcePJ.context.Unlock()
	}

	if targetPJ.context != c && targetPJ.context != sourcePJ.context {
		targetPJ.context.Lock()
		defer targetPJ.context.Unlock()
	}

	var cOptionsPtr **C.char
	if options != "" {
		cOptions := C.CString(options)
		defer C.free(unsafe.Pointer(cOptions))
		cOptionsPtr = &cOptions
	}

	var cArea *C.PJ_AREA
	if area != nil {
		cArea = area.pjArea
	}

	return c.newPJ(C.proj_create_crs_to_crs_from_pj(c.pjContext, sourcePJ.pj, targetPJ.pj, cArea, cOptionsPtr))
}

// New returns a new PJ with the given definition.
func (c *Context) New(definition string) (*PJ, error) {
	c.Lock()
	defer c.Unlock()

	cDefinition := C.CString(definition)
	defer C.free(unsafe.Pointer(cDefinition))

	return c.newPJ(C.proj_create(c.pjContext, cDefinition))
}

// NewFromArgs returns a new PJ from args.
func (c *Context) NewFromArgs(args ...string) (*PJ, error) {
	c.Lock()
	defer c.Unlock()

	cArgs := make([]*C.char, len(args))
	for i := range cArgs {
		cArg := C.CString(args[i])
		defer C.free(unsafe.Pointer(cArg))
		cArgs[i] = cArg
	}

	return c.newPJ(C.proj_create_argv(c.pjContext, (C.int)(len(cArgs)), (**C.char)(unsafe.Pointer(&cArgs[0]))))
}

// PJ PROJ_DLL *proj_create_compound_crs(PJ_CONTEXT *ctx, const char *crs_name,
//                                       PJ *horiz_crs, PJ *vert_crs);

func (c *Context) CreateCompoundCrs(name string, horizontalPJ *PJ, verticalPJ *PJ) (*PJ, error) {
	c.Lock()
	defer c.Unlock()

	if horizontalPJ.context != c {
		horizontalPJ.context.Lock()
		defer horizontalPJ.context.Unlock()
	}

	if verticalPJ.context != c && verticalPJ.context != horizontalPJ.context {
		verticalPJ.context.Lock()
		defer verticalPJ.context.Unlock()
	}

	var cName *C.char
	// Proj documentation states this field may be NULL, so we only pass a
	// non-null pointer if the text is not empty
	if len(name) > 0 {
		cName := C.CString(name)
		defer C.free(unsafe.Pointer(cName))
	}

	return c.newPJ(C.proj_create_compound_crs(c.pjContext, cName, horizontalPJ.pj, verticalPJ.pj))
}

func (c *Context) Unlock() {
	c.mutex.Unlock()
}

func (c *Context) GetAuthoritiesFromDatabase() ([]string, error) {
	c.Lock()
	defer c.Unlock()

	cAuthorities := C.proj_get_authorities_from_database(c.pjContext)
	if err := c.checkError(); err != nil {
		return nil, err
	}
	defer C.proj_string_list_destroy(cAuthorities)

	authorities := nullTerminatedListToGoSlice(cAuthorities)
	return authorities, nil
}

func (c *Context) GetAllCRSCodes() ([]string, error) {
	c.Lock()
	defer c.Unlock()

	cAuthorities := C.proj_get_authorities_from_database(c.pjContext)
	if err := c.checkError(); err != nil {
		return nil, fmt.Errorf("failed to list authorities from database: %w", err)
	}
	defer C.proj_string_list_destroy(cAuthorities)

	authorities := nullTerminatedListToGoSlice(cAuthorities)

	codeList := make([]string, 0)
	for _, auth := range authorities {
		cAuth := C.CString(auth)
		defer C.free(unsafe.Pointer(cAuth))

		cCodes := C.proj_get_codes_from_database(
			c.pjContext,
			cAuth,
			C.PJ_TYPE_CRS,
			0,
		)
		if err := c.checkError(); err != nil {
			return nil, fmt.Errorf("failed to list codes for authority %s: %w", auth, err)
		}
		defer C.proj_string_list_destroy(cCodes)

		codes := nullTerminatedListToGoSlice(cCodes)

		for _, code := range codes {
			codeList = append(codeList, fmt.Sprintf("%s:%s", auth, code))
		}
	}
	return codeList, nil
}

// errnoString returns the text representation of errno.
func (c *Context) errnoString(errno int) string {
	c.Lock()
	defer c.Unlock()
	return C.GoString(C.proj_context_errno_string(c.pjContext, (C.int)(errno)))
}

// newError returns a new error with number errno.
func (c *Context) newError(errno int) *Error {
	return &Error{
		context: c,
		errno:   errno,
	}
}

// newPJ returns a new PJ or an error.
func (c *Context) newPJ(cPJ *C.PJ) (*PJ, error) {
	if cPJ == nil {
		return nil, c.newError(int(C.proj_context_errno(c.pjContext)))
	}

	pj := &PJ{
		context: c,
		pj:      cPJ,
	}
	runtime.SetFinalizer(pj, (*PJ).Destroy)
	return pj, nil
}

func (c *Context) checkError() error {
	errno := int(C.proj_context_errno(c.pjContext))
	if errno == 0 {
		return nil
	}
	return c.newError(errno)
}

// SetLogLevel sets the log level for the default context.
func SetLogLevel(logLevel LogLevel) {
	defaultContext.SetLogLevel(logLevel)
}

// New returns a PJ with the given definition.
func New(definition string) (*PJ, error) {
	return defaultContext.New(definition)
}

// NewFromArgs returns a PJ with the given args.
func NewFromArgs(args ...string) (*PJ, error) {
	return defaultContext.NewFromArgs(args...)
}

// NewCRSToCRS returns a new PJ from sourceCRS to targetCRS and optional area.
func NewCRSToCRS(sourceCRS, targetCRS string, area *Area) (*PJ, error) {
	return defaultContext.NewCRSToCRS(sourceCRS, targetCRS, area)
}

// NewCRSToCRSFromPJ returns a new PJ from two CRSs.
func NewCRSToCRSFromPJ(sourcePJ, targetPJ *PJ, area *Area, options string) (*PJ, error) {
	return defaultContext.NewCRSToCRSFromPJ(sourcePJ, targetPJ, area, options)
}

// CreateCompoundCrs creates a compound CRS from two individual PJ objects
// The name can be the name of the GeographicCRS or empty
func CreateCompoundCrs(name string, horizontalPJ *PJ, verticalPJ *PJ) (*PJ, error) {
	return defaultContext.CreateCompoundCrs(name, horizontalPJ, verticalPJ)
}

func GetAuthoritiesFromDatabase() ([]string, error) {
	return defaultContext.GetAuthoritiesFromDatabase()
}

func GetAllCRSCodes() ([]string, error) {
	return defaultContext.GetAllCRSCodes()
}

func nullTerminatedListToGoSlice(res **C.char) []string {
	goStrings := make([]string, 0)
	for {
		if res == nil {
			break
		}

		cstr := *res
		if cstr == nil {
			break
		}

		goStrings = append(goStrings, C.GoString(cstr))

		res = (**C.char)(unsafe.Pointer(uintptr(unsafe.Pointer(res)) + unsafe.Sizeof(cstr)))
	}
	return goStrings
}
