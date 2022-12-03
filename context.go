package proj

// #include <stdlib.h>
// #include "go-proj.h"
import "C"

import (
	"runtime"
	"sync"
	"unsafe"
)

var defaultContext = &Context{}

// A Context is a context.
type Context struct {
	sync.Mutex
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

	cArgs := make([]*C.char, 0, len(args))
	for _, arg := range args {
		cArg := C.CString(arg)
		defer C.free(unsafe.Pointer(cArg))
		cArgs = append(cArgs, cArg)
	}

	return c.newPJ(C.proj_create_argv(c.pjContext, (C.int)(len(cArgs)), (**C.char)(unsafe.Pointer(&cArgs[0]))))
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

// NewCRSToCRS returns a new PJ from sourceCRS to targetCRS and optional area.
func NewCRSToCRS(sourceCRS, targetCRS string, area *Area) (*PJ, error) {
	return defaultContext.NewCRSToCRS(sourceCRS, targetCRS, area)
}
