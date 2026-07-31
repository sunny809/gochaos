// Package report serializes chaos evidence from the fault injection log into
// CI-consumable formats (JUnit XML and JSON).
package report

import (
	"encoding/xml"
	"fmt"
	"time"

	"github.com/sunny809/gochaos/internal/spec"
)

// JUnit renders the fault injection log as a JUnit XML test suite. Each
// injection becomes a failing testcase named "<faultType> on <stubID>", so the
// report reads as chaos evidence in CI dashboards: tests == injections.
func JUnit(entries []spec.FaultInjectionEntry, suiteName string) []byte {
	type failure struct {
		Message string `xml:"message,attr"`
		Type    string `xml:"type,attr"`
		Body    string `xml:",chardata"`
	}
	type testcase struct {
		Name      string    `xml:"name,attr"`
		ClassName string    `xml:"classname,attr"`
		Failures  []failure `xml:"failure,omitempty"`
	}
	type testsuite struct {
		XMLName  xml.Name   `xml:"testsuite"`
		Name     string     `xml:"name,attr"`
		Tests    int        `xml:"tests,attr"`
		Failures int        `xml:"failures,attr"`
		Cases    []testcase `xml:"testcase"`
	}

	suite := testsuite{Name: suiteName, Tests: len(entries), Failures: len(entries)}
	for _, e := range entries {
		faultType := e.FaultType
		if faultType == "" {
			faultType = "delay"
		}
		suite.Cases = append(suite.Cases, testcase{
			Name:      faultType,
			ClassName: e.StubID,
			Failures: []failure{{
				Message: fmt.Sprintf("fault injected on %s %s at %s",
					e.RequestMethod, e.RequestPath, e.ActivatedAt.Format(time.RFC3339)),
				Type: faultType,
				Body: faultType,
			}},
		})
	}

	out, _ := xml.MarshalIndent(suite, "", "  ")
	return append([]byte(xml.Header), out...)
}
