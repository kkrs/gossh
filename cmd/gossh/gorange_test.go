package main

import (
	"reflect"
	"sort"
	"testing"
)

var expandSets = []struct {
	inp string
	res []string
}{
	{
		"adma1100-2.dom.tld",
		[]string{
			"adma1100.dom.tld",
			"adma1101.dom.tld",
			"adma1102.dom.tld",
		},
	},
	{
		"adma1000-02.dom.tld",
		[]string{
			"adma1000.dom.tld",
			"adma1001.dom.tld",
			"adma1002.dom.tld",
		},
	},
	{
		"a10-1.dom.tld,b10-1.dom.tld,a-b.dom.tld",
		[]string{
			"a10.dom.tld",
			"a11.dom.tld",
			"b10.dom.tld",
			"b11.dom.tld",
			"a-b.dom.tld",
		},
	},
}

func TestExpand(t *testing.T) {
	for _, e := range expandSets {
		res, err := expand(e.inp)
		if err != nil {
			t.Error(err)
		}
		sort.Strings(e.res)
		sort.Strings(res)
		if !reflect.DeepEqual(res, e.res) {
			t.Error("For", e.inp, "expected", e.res, "got", res)
		}
	}
}

var compressSets = []struct {
	inp []string
	res string
}{
	{[]string{"b100.dom.tld", "b101.dom.tld"}, "b100-101.dom.tld"},
	{[]string{"b101.dom.tld", "b100.dom.tld"}, "b100-101.dom.tld"},
	{[]string{"b11.dom.tld", "b100.dom.tld", "b10.dom.tld"}, "{b10-11,b100}.dom.tld"},
	{[]string{"b0011.dom.tld", "b0101.dom.tld", "b0010.dom.tld"}, "{b0010-11,b0101}.dom.tld"},
}

func TestCompress(t *testing.T) {
	for _, e := range compressSets {
		res := compress(e.inp)
		if res != e.res {
			t.Error("For", e.inp, "expected", e.res, "got", res)
		}
	}
}
