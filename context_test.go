package proj_test

import (
	_ "embed"
	"runtime"
	"strconv"
	"testing"

	"github.com/alecthomas/assert/v2"

	"github.com/michiho/go-proj/v10"
)

func TestContext_NewCRSToCRS(t *testing.T) {
	defer runtime.GC()

	context := proj.NewContext()
	assert.NotZero(t, context)

	for _, tc := range []struct {
		name        string
		sourceCRS   string
		targetCRS   string
		expectedErr map[int]string
	}{
		{
			name:      "EPSG:4326_to_EPSG;3857",
			sourceCRS: "EPSG:4326",
			targetCRS: "EPSG:3857",
		},
		{
			name:      "EPSG:4326_to_invalid",
			sourceCRS: "EPSG:4326",
			targetCRS: "invalid",
			expectedErr: map[int]string{
				6: "generic error of unknown origin",
				8: "Unknown error (code 4096)",
				9: "Invalid PROJ string syntax",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pj, err := context.NewCRSToCRS(tc.sourceCRS, tc.targetCRS, nil)
			if tc.expectedErr != nil {
				assert.EqualError(t, err, tc.expectedErr[proj.VersionMajor])
				assert.Zero(t, pj)
			} else {
				assert.NoError(t, err)
				assert.NotZero(t, pj)
			}
		})
	}
}

func TestContext_NewCRSToCRSFromPJ(t *testing.T) {
	defer runtime.GC()

	sourceCRS, err := proj.New("epsg:4326")
	assert.NoError(t, err)
	assert.True(t, sourceCRS.IsCRS())

	targetCRS, err := proj.New("epsg:3857")
	assert.NoError(t, err)
	assert.True(t, targetCRS.IsCRS())

	pj, err := proj.NewCRSToCRSFromPJ(sourceCRS, targetCRS, nil, "")
	assert.NoError(t, err)
	assert.NotZero(t, pj)
}

func TestContext_New(t *testing.T) {
	defer runtime.GC()

	context := proj.NewContext()
	assert.NotZero(t, context)

	for _, tc := range []struct {
		definition  string
		expectedErr map[int]string
	}{
		{
			definition: "epsg:4326",
		},
		{
			definition: "+proj=etmerc +lat_0=38 +lon_0=125 +ellps=bessel",
		},
		{
			definition: "invalid",
			expectedErr: map[int]string{
				6: "generic error of unknown origin",
				8: "Unknown error (code 4096)",
				9: "Invalid PROJ string syntax",
			},
		},
	} {
		t.Run(tc.definition, func(t *testing.T) {
			pj, err := context.New(tc.definition)
			if tc.expectedErr != nil {
				assert.EqualError(t, err, tc.expectedErr[proj.VersionMajor])
				assert.Zero(t, pj)
			} else {
				assert.NoError(t, err)
				assert.NotZero(t, pj)
			}
		})
	}
}

func TestContext_NewFromArgs(t *testing.T) {
	defer runtime.GC()

	context := proj.NewContext()
	assert.NotZero(t, context)

	for i, tc := range []struct {
		args        []string
		expectedErr map[int]string
	}{
		{
			args: []string{"proj=utm", "zone=32", "ellps=GRS80"},
		},
		{
			args: []string{"proj=utm", "zone=0", "ellps=GRS80"},
			expectedErr: map[int]string{
				6: "invalid UTM zone number",
				8: "Invalid value for an argument",
				9: "Invalid value for an argument",
			},
		},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			pj, err := context.NewFromArgs(tc.args...)
			if tc.expectedErr != nil {
				assert.EqualError(t, err, tc.expectedErr[proj.VersionMajor])
				assert.Zero(t, pj)
			} else {
				assert.NoError(t, err)
				assert.NotZero(t, pj)
			}
		})
	}
}

func TestContext_SetSearchPaths(t *testing.T) {
	defer runtime.GC()

	context := proj.NewContext()
	assert.NotZero(t, context)

	// The C function does not return any error so we only validate
	// that executing the SetSearchPaths function call
	// does not panic considering various boundary conditions
	context.SetSearchPaths(nil)
	context.SetSearchPaths([]string{})
	context.SetSearchPaths([]string{"/tmp/data"})
	context.SetSearchPaths([]string{"/tmp/data", "/tmp/data2"})
}

func Test_GetAuthoritiesFromDatabase(t *testing.T) {
	res, err := proj.GetAuthoritiesFromDatabase()
	if err != nil {
		t.Fatalf("error %s", err.Error())
	}

	assertSliceEqualOrderInvariant(t, []string{
		"EPSG", "ESRI", "IAU_2015", "IGNF", "NKG", "PROJ", "OGC", "NRCAN",
	}, res)
}

func Test_GetAllCRSCodes(t *testing.T) {
	res, err := proj.GetAllCRSCodes()
	if err != nil {
		t.Fatalf("error %s", err.Error())
	}
	expectedLengths := map[int]int{
		8: 10889,
		9: 11615,
	}

	assert.Equal(t, expectedLengths[proj.VersionMajor], len(res), "unexpected length of codes")
}

func Test_CreateCompoundCrs(t *testing.T) {
	horiz, err := proj.New("EPSG:4326")
	assert.NoError(t, err, "failed to create horizontal part")

	// EVRF2007 height (Europe) EPSG:5621
	vert, err := proj.New(`VERT_CS["EVRF2007 height",
    VERT_DATUM["European Vertical Reference Frame 2007",2005,
        AUTHORITY["EPSG","5215"]],
    UNIT["metre",1,
        AUTHORITY["EPSG","9001"]],
    AXIS["Gravity-related height",UP],
    AUTHORITY["EPSG","5621"]]`)
	assert.NoError(t, err, "failed to create vertical part")

	compound, err := proj.CreateCompoundCrs("", horiz, vert)
	assert.NoError(t, err, "failed to create compound CRS")

	otherCompound, err := proj.New("EPSG:4326+9390")
	assert.NoError(t, err, "failed to create other compound CRS")

	transform, err := proj.NewCRSToCRSFromPJ(compound, otherCompound, nil, "")
	assert.NoError(t, err, "failed to get transformation between both CRS")

	_, err = transform.Forward(proj.NewCoord(10, 54, 42, 0))
	assert.NoError(t, err, "failed to transform some coord")
}

func assertSliceEqualOrderInvariant(tb testing.TB, expected, actual []string) {
	tb.Helper()

	expectedItems := make(map[string]int, len(expected))
	for _, item := range expected {
		count, ok := expectedItems[item]
		if !ok {
			expectedItems[item] = 1
		} else {
			expectedItems[item] = count + 1
		}
	}

	for _, item := range actual {
		remainingCount, ok := expectedItems[item]
		if !ok {
			tb.Errorf("unexpected item %s", item)
		}

		expectedItems[item] = remainingCount - 1
	}

	for item, remainingCount := range expectedItems {
		if remainingCount > 0 {
			tb.Errorf("expected %d more occurrences of %s", remainingCount, item)
		}
		if remainingCount < 0 {
			tb.Errorf("expected %d less occurrences of %s", -remainingCount, item)
		}
	}
	assert.Equal(tb, len(expected), len(actual))

}
