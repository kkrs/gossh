// main is a library to expand and compress lists of hostnames using the range syntax. This is
// implemented as a simple wrapper around github.com/xaviershay/grange
package main

import (
	"regexp"

	"github.com/xaviershay/grange"
	"gopkg.in/deckarep/v1/golang-set"
)

func expand(rangeStr string) ([]string, error) {
	grange.MaxQuerySize = 10000
	s := grange.NewState()
	re := regexp.MustCompile(`(\d+)-(\d+)`)
	rangeStr = re.ReplaceAllString(rangeStr, "${1}..${2}")
	res, err := s.Query(rangeStr)
	if err != nil {
		return nil, err
	}
	var result []string
	for rI := range res.Iter() {
		r, _ := rI.(string)
		result = append(result, r)
	}
	return result, nil
}

func compress(nodes []string) string {
	grange.MaxQuerySize = 10000
	var nodesI = make([]interface{}, len(nodes))
	for i, n := range nodes {
		nodesI[i] = interface{}(n)
	}
	result := grange.Result{mapset.NewSetFromSlice(nodesI)}
	// convert the result back to use '-'
	oRes := grange.Compress(&result)
	re := regexp.MustCompile(`(\d+)\.\.(\d+)`)
	return re.ReplaceAllString(oRes, "${1}-${2}")
}
