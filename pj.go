package proj

// #include "go-proj.h"
import "C"

import (
	"fmt"
	"unsafe"
)

// A Direction is a direction.
type Direction C.PJ_DIRECTION

// Directions.
const (
	DirectionFwd   Direction = C.PJ_FWD
	DirectionIdent Direction = C.PJ_IDENT
	DirectionInv   Direction = C.PJ_INV
)

// A PJ is a projection or a transformation.
type PJ struct {
	context *Context
	pj      *C.PJ
}

// A PJInfo contains information about a PJ.
type PJInfo struct {
	ID          string  // Short ID of the operation the PJ object is based on, that is, what comes after the +proj= in a proj-string, e.g. "merc".
	Description string  // Long describes of the operation the PJ object is based on, e.g. "Mercator Cyl, Sph&Ell lat_ts=".
	Definition  string  // The proj-string that was used to create the PJ object with, e.g. "+proj=merc +lat_0=24 +lon_0=53 +ellps=WGS84".
	HasInverse  bool    // True if an inverse mapping of the defined operation exists.
	Accuracy    float64 // Expected accuracy of the transformation. -1 if unknown.
}

type SRID struct {
	Auth string
	Code string
}

func (s SRID) String() string {
	if s.Auth == "" && s.Code == "" {
		return ""
	}
	return s.Auth + ":" + s.Code
}

// FullPJInfo is a shorthand to contain more info about a PJ from different C methods
type FullPJInfo struct {
	PJInfo
	IsCrs      bool // True if this is a CRS
	Type       PJType
	CrsMatches []IdentifyMatchInfo // If this is a CRS, this list contains all matches from the database
	AreaOfUse  *AreaOfUse
}

type IdentifyMatchInfo struct {
	SRID        SRID
	Description string
	Confidence  int
}

// A match from the Identify() method. For the meanings of confidence consult
// https://proj.org/en/stable/development/reference/functions.html#c.proj_identify
type IdentifyMatch struct {
	PJ         *PJ
	Confidence int
}

// Destroy releases all resources associated with pj.
func (pj *PJ) Destroy() {
	pj.context.Lock()
	defer pj.context.Unlock()
	if pj.pj != nil {
		C.proj_destroy(pj.pj)
		pj.pj = nil
	}
}

// Returns a new PJ instance whose axis order is the one expected for
// visualization purposes. If the axis order of its source or target CRS is
// northing, easting, then an axis swap operation will be inserted.
//
// The axis order of geographic CRS will be longitude, latitude[, height], and
// the one of projected CRS will be easting, northing [, height].
func (pj *PJ) NormalizeForVisualization() (*PJ, error) {
	pj.context.Lock()
	defer pj.context.Unlock()
	return pj.context.newPJ(C.proj_normalize_for_visualization(pj.context.pjContext, pj.pj))
}

// Forward transforms coord in the forward direction.
func (pj *PJ) Forward(coord Coord) (Coord, error) {
	return pj.Trans(DirectionFwd, coord)
}

// ForwardBounds transforms bounds in the forward direction.
func (pj *PJ) ForwardBounds(bounds Bounds, densifyPoints int) (Bounds, error) {
	return pj.TransBounds(DirectionFwd, bounds, densifyPoints)
}

// ForwardArray transforms coords in the forward direction.
func (pj *PJ) ForwardArray(coords []Coord) error {
	return pj.TransArray(DirectionFwd, coords)
}

// ForwardFlatCoords transforms flatCoords in the forward direction.
func (pj *PJ) ForwardFlatCoords(flatCoords []float64, stride, zIndex, mIndex int) error {
	return pj.TransFlatCoords(DirectionFwd, flatCoords, stride, zIndex, mIndex)
}

// ForwardFloat64Slice transforms float64 in place in the forward direction.
func (pj *PJ) ForwardFloat64Slice(float64Slice []float64) ([]float64, error) {
	return pj.TransFloat64Slice(DirectionFwd, float64Slice)
}

// ForwardFloat64Slices transforms float64Slices in the forward direction.
func (pj *PJ) ForwardFloat64Slices(float64Slices [][]float64) error {
	return pj.TransFloat64Slices(DirectionFwd, float64Slices)
}

// Geod returns the distance, forward azimuth, and reverse azimuth between a and b.
func (pj *PJ) Geod(a, b Coord) (float64, float64, float64) {
	pj.context.Lock()
	defer pj.context.Unlock()
	cCoord := C.proj_geod(pj.pj, *(*C.PJ_COORD)(unsafe.Pointer(&a)), *(*C.PJ_COORD)(unsafe.Pointer(&b)))
	cGeod := *(*C.PJ_GEOD)(unsafe.Pointer(&cCoord))
	return (float64)(cGeod.s), (float64)(cGeod.a1), (float64)(cGeod.a2)
}

// GetLastUsedOperation returns the operation used in the last call to Trans.
func (pj *PJ) GetLastUsedOperation() (*PJ, error) {
	pj.context.Lock()
	defer pj.context.Unlock()
	return pj.context.newPJ(C.proj_trans_get_last_used_operation(pj.pj))
}

// Info returns information about pj.
func (pj *PJ) Info() PJInfo {
	pj.context.Lock()
	defer pj.context.Unlock()

	cProjInfo := C.proj_pj_info(pj.pj)
	return PJInfo{
		ID:          C.GoString(cProjInfo.id),
		Description: C.GoString(cProjInfo.description),
		Definition:  C.GoString(cProjInfo.definition),
		HasInverse:  cProjInfo.has_inverse != 0,
		Accuracy:    (float64)(cProjInfo.accuracy),
	}
}

// FullInfo is a convenience method combining other methods to get plenty of
// info on a PJ object.
func (pj *PJ) FullInfo() (*FullPJInfo, error) {
	var err error
	result := &FullPJInfo{}

	result.PJInfo = pj.Info()
	result.IsCrs = pj.IsCRS()
	result.Type, err = pj.GetType()
	result.AreaOfUse = pj.GetAreaOfUse()
	if err != nil {
		return nil, fmt.Errorf("failed to get PJ type: %w", err)
	}

	if result.IsCrs {
		matches, err := pj.Identify()
		if err != nil {
			return nil, fmt.Errorf("failed to identify CRS: %w", err)
		}

		for _, m := range matches {
			result.CrsMatches = append(result.CrsMatches, IdentifyMatchInfo{
				SRID:        m.PJ.GetSRID(),
				Description: m.PJ.Info().Description,
				Confidence:  m.Confidence,
			})
		}
	}

	return result, nil
}

// Identify tries to match the given pj against the database of known CRS'
// and returns matches.
// TODO: implement restricting authorities when needed
func (pj *PJ) Identify() ([]IdentifyMatch, error) {
	var confidenceList *C.int
	objList, err := pj.identifyRaw(&confidenceList)
	if err != nil {
		return nil, fmt.Errorf("failed to proj_identify: %w", err)
	}

	defer C.proj_list_destroy(objList)
	defer C.proj_int_list_destroy(confidenceList)

	matches, err := readPjList(objList, pj)
	if err != nil {
		return nil, fmt.Errorf("failed to read PJ_OBJ_LIST to go slice: %w", err)
	}

	confidenceSlice := (*[1 << 30]C.int)(unsafe.Pointer(confidenceList))[:len(matches):len(matches)]

	result := make([]IdentifyMatch, 0, len(matches))
	for i, match := range matches {
		result = append(result, IdentifyMatch{
			PJ:         match,
			Confidence: int(confidenceSlice[i]),
		})
	}

	return result, nil
}

func (pj *PJ) ListSubCRS() ([]*PJ, error) {
	pj.context.Lock()
	defer pj.context.Unlock()

	var result []*PJ
	finished := false
	maxTries := 100
	for i := 0; i < maxTries; i++ {
		newPj, err := pj.GetSubCRS(i)
		if err != nil {
			return nil, err
		}

		if newPj == nil {
			finished = true
			break
		}

		result = append(result, newPj)
	}
	if !finished {
		return nil, fmt.Errorf("listing sub-crs aborted after %d runs", maxTries)
	}

	return result, nil
}

func (pj *PJ) GetSubCRS(index int) (*PJ, error) {
	lastErrno := C.proj_errno_reset(pj.pj)
	defer C.proj_errno_restore(pj.pj, lastErrno)

	rawPj := C.proj_crs_get_sub_crs(pj.context.pjContext, pj.pj, C.int(index))
	if errno := int(C.proj_errno(pj.pj)); errno != 0 {
		return nil, fmt.Errorf("failed to get sub-crs %d: %w", index, pj.context.newError(errno))
	}

	if rawPj == nil {
		return nil, nil
	}

	newPj, err := pj.context.newPJ(rawPj)
	if err != nil {
		return nil, fmt.Errorf("failed to create sub-pj %d: %w", index, err)
	}
	return newPj, nil
}

func (pj *PJ) identifyRaw(confidenceList **C.int) (*C.PJ_OBJ_LIST, error) {
	pj.context.Lock()
	defer pj.context.Unlock()

	lastErrno := C.proj_errno_reset(pj.pj)
	defer C.proj_errno_restore(pj.pj, lastErrno)

	objList := C.proj_identify(pj.context.pjContext, pj.pj, nil, nil, confidenceList)
	if errno := int(C.proj_errno(pj.pj)); errno != 0 {
		return nil, pj.context.newError(errno)
	}

	return objList, nil
}

func readPjList(list *C.PJ_OBJ_LIST, referencePj *PJ) ([]*PJ, error) {
	count := int(C.proj_list_get_count(list))

	result := make([]*PJ, 0, count)
	for i := 0; i < count; i++ {
		rawPj, err := projListGet(list, i, referencePj)
		if err != nil {
			return nil, fmt.Errorf("failed to get item %d from PJ_OBJ_LIST: %w", i, err)
		}

		newPj, err := referencePj.context.newPJ(rawPj)
		if err != nil {
			return nil, fmt.Errorf("failed to convert item %d to go PJ type: %w", i, err)
		}

		result = append(result, newPj)
	}
	return result, nil
}

func projListGet(list *C.PJ_OBJ_LIST, index int, referencePj *PJ) (*C.PJ, error) {
	lastErrno := C.proj_errno_reset(referencePj.pj)
	defer C.proj_errno_restore(referencePj.pj, lastErrno)

	p := C.proj_list_get(referencePj.context.pjContext, list, C.int(index))
	if errno := int(C.proj_errno(referencePj.pj)); errno != 0 {
		return nil, referencePj.context.newError(errno)
	}

	return p, nil
}

// GetSRID returns the Spatial Reference Identifier, like "EPSG","4326", for pj.
func (pj *PJ) GetSRID() SRID {
	pj.context.Lock()
	defer pj.context.Unlock()

	auth := C.proj_get_id_auth_name(pj.pj, 0)

	code := C.proj_get_id_code(pj.pj, 0)
	return SRID{
		Auth: C.GoString(auth),
		Code: C.GoString(code),
	}
}

// IsCRS returns whether pj is a CRS.
func (pj *PJ) IsCRS() bool {
	return C.proj_is_crs(pj.pj) != 0
}

type WKTType C.PJ_WKT_TYPE

const (
	PJ_WKT2_2015            WKTType = C.PJ_WKT2_2015
	PJ_WKT2_2015_SIMPLIFIED WKTType = C.PJ_WKT2_2015_SIMPLIFIED
	PJ_WKT2_2019            WKTType = C.PJ_WKT2_2019
	PJ_WKT2_2018            WKTType = C.PJ_WKT2_2018
	PJ_WKT2_2019_SIMPLIFIED WKTType = C.PJ_WKT2_2019_SIMPLIFIED
	PJ_WKT2_2018_SIMPLIFIED WKTType = C.PJ_WKT2_2018_SIMPLIFIED
	PJ_WKT1_GDAL            WKTType = C.PJ_WKT1_GDAL
	PJ_WKT1_ESRI            WKTType = C.PJ_WKT1_ESRI
)

func (t WKTType) ToString() string {
	switch t {
	case PJ_WKT2_2015:
		return "PJ_WKT2_2015"
	case PJ_WKT2_2015_SIMPLIFIED:
		return "PJ_WKT2_2015_SIMPLIFIED"
	case PJ_WKT2_2018:
		return "PJ_WKT2_2018"
	case PJ_WKT2_2018_SIMPLIFIED:
		return "PJ_WKT2_2018_SIMPLIFIED"
	case PJ_WKT1_GDAL:
		return "PJ_WKT1_GDAL"
	case PJ_WKT1_ESRI:
		return "PJ_WKT1_ESRI"
	default:
		return ""
	}
}

// TODO: implement those options in AsWkt()
// type AsWktOptions struct {
// 	Multiline                           bool   // MULTILINE
// 	IndentationWidth                    uint   // INDENTATION_WIDTH
// 	OutputAxis                          string // OUTPUT_AXIS
// 	Strict                              bool   // STRICT
// 	AllowEllipsoidalHeightAsVerticalCrs bool   // ALLOW_ELLIPSOIDAL_HEIGHT_AS_VERTICAL_CRS
// 	AllowLinunitNode                    bool   // ALLOW_LINUNIT_NODE
// }

// AsWkt gets a WKT representation of an object. Defaults to Multiline output
// with indentation level of 4. Options to modify that may be added to this binding
// if needed
func (pj *PJ) AsWkt(wktType WKTType) (string, error) {
	pj.context.Lock()
	defer pj.context.Unlock()

	lastErrno := C.proj_errno_reset(pj.pj)
	defer C.proj_errno_restore(pj.pj, lastErrno)

	wkt := C.proj_as_wkt(pj.context.pjContext, pj.pj, C.PJ_WKT_TYPE(wktType), nil)
	if errno := int(C.proj_errno(pj.pj)); errno != 0 {
		return "", pj.context.newError(errno)
	}

	if wkt == nil {
		return "", fmt.Errorf("projection not compatible with an export to %s", wktType.ToString())
	}

	return C.GoString(wkt), nil
}

type PJType string

const (
	PJ_TYPE_UNKNOWN                          PJType = "UNKNOWN"
	PJ_TYPE_ELLIPSOID                        PJType = "ELLIPSOID"
	PJ_TYPE_PRIME_MERIDIAN                   PJType = "PRIME_MERIDIAN"
	PJ_TYPE_GEODETIC_REFERENCE_FRAME         PJType = "GEODETIC_REFERENCE_FRAME"
	PJ_TYPE_DYNAMIC_GEODETIC_REFERENCE_FRAME PJType = "DYNAMIC_GEODETIC_REFERENCE_FRAME"
	PJ_TYPE_VERTICAL_REFERENCE_FRAME         PJType = "VERTICAL_REFERENCE_FRAME"
	PJ_TYPE_DYNAMIC_VERTICAL_REFERENCE_FRAME PJType = "DYNAMIC_VERTICAL_REFERENCE_FRAME"
	PJ_TYPE_DATUM_ENSEMBLE                   PJType = "DATUM_ENSEMBLE"

	// Abstract type, not returned by proj_get_type()
	PJ_TYPE_CRS PJType = "CRS"

	PJ_TYPE_GEODETIC_CRS   PJType = "GEODETIC_CRS"
	PJ_TYPE_GEOCENTRIC_CRS PJType = "GEOCENTRIC_CRS"

	// Abstract type, not returned by proj_get_type()
	PJ_TYPE_GEOGRAPHIC_CRS PJType = "GEOGRAPHIC_CRS"

	PJ_TYPE_GEOGRAPHIC_2D_CRS PJType = "GEOGRAPHIC_2D_CRS"
	PJ_TYPE_GEOGRAPHIC_3D_CRS PJType = "GEOGRAPHIC_3D_CRS"
	PJ_TYPE_VERTICAL_CRS      PJType = "VERTICAL_CRS"
	PJ_TYPE_PROJECTED_CRS     PJType = "PROJECTED_CRS"
	PJ_TYPE_COMPOUND_CRS      PJType = "COMPOUND_CRS"
	PJ_TYPE_TEMPORAL_CRS      PJType = "TEMPORAL_CRS"
	PJ_TYPE_ENGINEERING_CRS   PJType = "ENGINEERING_CRS"
	PJ_TYPE_BOUND_CRS         PJType = "BOUND_CRS"
	PJ_TYPE_OTHER_CRS         PJType = "OTHER_CRS"

	PJ_TYPE_CONVERSION                 PJType = "CONVERSION"
	PJ_TYPE_TRANSFORMATION             PJType = "TRANSFORMATION"
	PJ_TYPE_CONCATENATED_OPERATION     PJType = "CONCATENATED_OPERATION"
	PJ_TYPE_OTHER_COORDINATE_OPERATION PJType = "OTHER_COORDINATE_OPERATION"

	PJ_TYPE_TEMPORAL_DATUM    PJType = "TEMPORAL_DATUM"
	PJ_TYPE_ENGINEERING_DATUM PJType = "ENGINEERING_DATUM"
	PJ_TYPE_PARAMETRIC_DATUM  PJType = "PARAMETRIC_DATUM"

	PJ_TYPE_DERIVED_PROJECTED_CRS PJType = "DERIVED_PROJECTED_CRS"

	PJ_TYPE_COORDINATE_METADATA PJType = "COORDINATE_METADATA"
)

func mapPJTypeFromC(cType C.PJ_TYPE) (PJType, error) {
	switch cType {

	case C.PJ_TYPE_UNKNOWN:
		return PJ_TYPE_UNKNOWN, nil

	case C.PJ_TYPE_ELLIPSOID:
		return PJ_TYPE_ELLIPSOID, nil

	case C.PJ_TYPE_PRIME_MERIDIAN:
		return PJ_TYPE_PRIME_MERIDIAN, nil

	case C.PJ_TYPE_GEODETIC_REFERENCE_FRAME:
		return PJ_TYPE_GEODETIC_REFERENCE_FRAME, nil
	case C.PJ_TYPE_DYNAMIC_GEODETIC_REFERENCE_FRAME:
		return PJ_TYPE_DYNAMIC_GEODETIC_REFERENCE_FRAME, nil
	case C.PJ_TYPE_VERTICAL_REFERENCE_FRAME:
		return PJ_TYPE_VERTICAL_REFERENCE_FRAME, nil
	case C.PJ_TYPE_DYNAMIC_VERTICAL_REFERENCE_FRAME:
		return PJ_TYPE_DYNAMIC_VERTICAL_REFERENCE_FRAME, nil
	case C.PJ_TYPE_DATUM_ENSEMBLE:
		return PJ_TYPE_DATUM_ENSEMBLE, nil
	case C.PJ_TYPE_CRS:
		return PJ_TYPE_CRS, nil

	case C.PJ_TYPE_GEODETIC_CRS:
		return PJ_TYPE_GEODETIC_CRS, nil
	case C.PJ_TYPE_GEOCENTRIC_CRS:
		return PJ_TYPE_GEOCENTRIC_CRS, nil
	case C.PJ_TYPE_GEOGRAPHIC_CRS:
		return PJ_TYPE_GEOGRAPHIC_CRS, nil

	case C.PJ_TYPE_GEOGRAPHIC_2D_CRS:
		return PJ_TYPE_GEOGRAPHIC_2D_CRS, nil
	case C.PJ_TYPE_GEOGRAPHIC_3D_CRS:
		return PJ_TYPE_GEOGRAPHIC_3D_CRS, nil
	case C.PJ_TYPE_VERTICAL_CRS:
		return PJ_TYPE_VERTICAL_CRS, nil
	case C.PJ_TYPE_PROJECTED_CRS:
		return PJ_TYPE_PROJECTED_CRS, nil
	case C.PJ_TYPE_COMPOUND_CRS:
		return PJ_TYPE_COMPOUND_CRS, nil
	case C.PJ_TYPE_TEMPORAL_CRS:
		return PJ_TYPE_TEMPORAL_CRS, nil
	case C.PJ_TYPE_ENGINEERING_CRS:
		return PJ_TYPE_ENGINEERING_CRS, nil
	case C.PJ_TYPE_BOUND_CRS:
		return PJ_TYPE_BOUND_CRS, nil
	case C.PJ_TYPE_OTHER_CRS:
		return PJ_TYPE_OTHER_CRS, nil

	case C.PJ_TYPE_CONVERSION:
		return PJ_TYPE_CONVERSION, nil
	case C.PJ_TYPE_TRANSFORMATION:
		return PJ_TYPE_TRANSFORMATION, nil
	case C.PJ_TYPE_CONCATENATED_OPERATION:
		return PJ_TYPE_CONCATENATED_OPERATION, nil
	case C.PJ_TYPE_OTHER_COORDINATE_OPERATION:
		return PJ_TYPE_OTHER_COORDINATE_OPERATION, nil

	case C.PJ_TYPE_TEMPORAL_DATUM:
		return PJ_TYPE_TEMPORAL_DATUM, nil
	case C.PJ_TYPE_ENGINEERING_DATUM:
		return PJ_TYPE_ENGINEERING_DATUM, nil
	case C.PJ_TYPE_PARAMETRIC_DATUM:
		return PJ_TYPE_PARAMETRIC_DATUM, nil

	case C.PJ_TYPE_DERIVED_PROJECTED_CRS:
		return PJ_TYPE_DERIVED_PROJECTED_CRS, nil

	case C.PJ_TYPE_COORDINATE_METADATA:
		return PJ_TYPE_COORDINATE_METADATA, nil

	default:
		return "", fmt.Errorf("unexpected PJ_TYPE: %d", cType)
	}
}

// GetType returns the type of the Projection
func (pj *PJ) GetType() (PJType, error) {
	pj.context.Lock()
	defer pj.context.Unlock()

	pjType := C.proj_get_type(pj.pj)
	return mapPJTypeFromC(pjType)
}

// AreaOfUse represents the geographic bounding box and area name.
type AreaOfUse struct {
	WestLon  float64
	SouthLat float64
	EastLon  float64
	NorthLat float64
	Name     string
}

// GetAreaOfUse retrieves the area of use or nil if it is unknown or an error occurred
func (pj *PJ) GetAreaOfUse() *AreaOfUse {
	pj.context.Lock()
	defer pj.context.Unlock()

	var west, south, east, north C.double
	var name *C.char

	success := C.proj_get_area_of_use(
		pj.context.pjContext,
		pj.pj,
		&west,
		&south,
		&east,
		&north,
		(**C.char)(&name),
	)

	if success == 0 {
		return nil
	}

	area := &AreaOfUse{
		WestLon:  float64(west),
		SouthLat: float64(south),
		EastLon:  float64(east),
		NorthLat: float64(north),
		Name:     C.GoString(name),
	}
	return area
}

// AsProjJson gives the definition of the PJ in ProjJson format
func (pj *PJ) AsProjJson() (string, error) {
	pj.context.Lock()
	defer pj.context.Unlock()

	lastErrno := C.proj_errno_reset(pj.pj)
	defer C.proj_errno_restore(pj.pj, lastErrno)

	projjson := C.proj_as_projjson(pj.context.pjContext, pj.pj, nil)
	if errno := int(C.proj_errno(pj.pj)); errno != 0 {
		return "", pj.context.newError(errno)
	}

	return C.GoString(projjson), nil
}

// Inverse transforms coord in the inverse direction.
func (pj *PJ) Inverse(coord Coord) (Coord, error) {
	return pj.Trans(DirectionInv, coord)
}

// InverseArray transforms coords in the inverse direction.
func (pj *PJ) InverseArray(coords []Coord) error {
	return pj.TransArray(DirectionInv, coords)
}

// InverseBounds transforms bounds in the forward direction.
func (pj *PJ) InverseBounds(bounds Bounds, densifyPoints int) (Bounds, error) {
	return pj.TransBounds(DirectionInv, bounds, densifyPoints)
}

// InverseFlatCoords transforms flatCoords in the inverse direction.
func (pj *PJ) InverseFlatCoords(flatCoords []float64, stride, zIndex, mIndex int) error {
	return pj.TransFlatCoords(DirectionInv, flatCoords, stride, zIndex, mIndex)
}

// InverseFloat64Slice transforms float64 in place in the forward direction.
func (pj *PJ) InverseFloat64Slice(float64Slice []float64) ([]float64, error) {
	return pj.TransFloat64Slice(DirectionInv, float64Slice)
}

// InverseFloat64Slices transforms float64Slices in the inverse direction.
func (pj *PJ) InverseFloat64Slices(float64Slices [][]float64) error {
	return pj.TransFloat64Slices(DirectionInv, float64Slices)
}

// LPDist returns the geodesic distance between a and b in geodetic coordinates.
func (pj *PJ) LPDist(a, b Coord) float64 {
	pj.context.Lock()
	defer pj.context.Unlock()
	return (float64)(C.proj_lp_dist(pj.pj, *(*C.PJ_COORD)(unsafe.Pointer(&a)), *(*C.PJ_COORD)(unsafe.Pointer(&b))))
}

// LPZDist returns the geodesic distance between a and b in geodetic
// coordinates, taking height above the ellipsoid into account.
func (pj *PJ) LPZDist(a, b Coord) float64 {
	pj.context.Lock()
	defer pj.context.Unlock()
	return (float64)(C.proj_lpz_dist(pj.pj, *(*C.PJ_COORD)(unsafe.Pointer(&a)), *(*C.PJ_COORD)(unsafe.Pointer(&b))))
}

// Trans transforms a single Coord in place.
func (pj *PJ) Trans(direction Direction, coord Coord) (Coord, error) {
	pj.context.Lock()
	defer pj.context.Unlock()

	lastErrno := C.proj_errno_reset(pj.pj)
	defer C.proj_errno_restore(pj.pj, lastErrno)

	pjCoord := C.proj_trans(pj.pj, (C.PJ_DIRECTION)(direction), *(*C.PJ_COORD)(unsafe.Pointer(&coord)))
	if errno := int(C.proj_errno(pj.pj)); errno != 0 {
		return Coord{}, pj.context.newError(errno)
	}
	return *(*Coord)(unsafe.Pointer(&pjCoord)), nil
}

// TransArray transforms an array of Coords.
func (pj *PJ) TransArray(direction Direction, coords []Coord) error {
	if len(coords) == 0 {
		return nil
	}

	pj.context.Lock()
	defer pj.context.Unlock()

	lastErrno := C.proj_errno_reset(pj.pj)
	defer C.proj_errno_restore(pj.pj, lastErrno)

	if errno := int(C.proj_trans_array(pj.pj, (C.PJ_DIRECTION)(direction), (C.size_t)(len(coords)), (*C.PJ_COORD)(unsafe.Pointer(&coords[0])))); errno != 0 {
		return pj.context.newError(errno)
	}
	return nil
}

// TransBounds transforms bounds.
func (pj *PJ) TransBounds(direction Direction, bounds Bounds, densifyPoints int) (Bounds, error) {
	pj.context.Lock()
	defer pj.context.Unlock()

	var transBounds Bounds
	if C.proj_trans_bounds(pj.context.pjContext, pj.pj, (C.PJ_DIRECTION)(direction),
		(C.double)(bounds.XMin), (C.double)(bounds.YMin), (C.double)(bounds.XMax), (C.double)(bounds.YMax),
		(*C.double)(&transBounds.XMin), (*C.double)(&transBounds.YMin), (*C.double)(&transBounds.XMax), (*C.double)(&transBounds.YMax),
		C.int(densifyPoints)) == 0 {
		return Bounds{}, pj.context.newError(int(C.proj_errno(pj.pj)))
	}
	return transBounds, nil
}

// TransFlatCoords transforms an array of flat coordinates.
func (pj *PJ) TransFlatCoords(direction Direction, flatCoords []float64, stride, zIndex, mIndex int) error {
	if len(flatCoords) == 0 {
		return nil
	}
	n := len(flatCoords) / stride

	var x, y, z, m *float64
	var sx, sy, sz, sm int
	var nx, ny, nz, nm int

	x = &flatCoords[0]
	y = &flatCoords[1]
	sx = 8 * stride
	sy = 8 * stride
	nx = n
	ny = n

	if zIndex != -1 {
		z = &flatCoords[zIndex]
		sz = 8 * stride
		nz = n
	}

	if mIndex != -1 {
		m = &flatCoords[mIndex]
		sm = 8 * stride
		nm = n
	}

	return pj.TransGeneric(direction, x, sx, nx, y, sy, ny, z, sz, nz, m, sm, nm)
}

// TransFloat64Slice transforms a []float64 in place.
func (pj *PJ) TransFloat64Slice(direction Direction, float64Slice []float64) ([]float64, error) {
	var coord Coord
	copy(coord[:], float64Slice)
	transCoord, err := pj.Trans(direction, coord)
	if err != nil {
		return nil, err
	}
	copy(float64Slice, transCoord[:])
	return float64Slice, nil
}

// TransFloat64Slices transforms float64Slices.
func (pj *PJ) TransFloat64Slices(direction Direction, float64Slices [][]float64) error {
	coords := Float64SlicesToCoords(float64Slices)
	if err := pj.TransArray(direction, coords); err != nil {
		return err
	}
	for i, coord := range coords {
		copy(float64Slices[i], coord[:])
	}
	return nil
}

// TransGeneric transforms a series of coordinates.
func (pj *PJ) TransGeneric(direction Direction, x *float64, sx, nx int, y *float64, sy, ny int, z *float64, sz, nz int, m *float64, sm, nm int) error {
	pj.context.Lock()
	defer pj.context.Unlock()

	lastErrno := C.proj_errno_reset(pj.pj)
	defer C.proj_errno_restore(pj.pj, lastErrno)

	if int(C.proj_trans_generic(pj.pj, (C.PJ_DIRECTION)(direction),
		(*C.double)(x), C.size_t(sx), C.size_t(nx),
		(*C.double)(y), C.size_t(sy), C.size_t(ny),
		(*C.double)(z), C.size_t(sz), C.size_t(nz),
		(*C.double)(m), C.size_t(sm), C.size_t(nm),
	)) != max(nx, ny, nz, nm) {
		return pj.context.newError(int(C.proj_errno(pj.pj)))
	}

	return nil
}
