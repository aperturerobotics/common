package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"

	compatibility "example.com/compatibility"
	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
)

type result struct {
	Wire             string   `json:"wire"`
	UnknownPreserved bool     `json:"unknown_preserved"`
	ParsedPresent    string   `json:"parsed_present"`
	ParsedText       string   `json:"parsed_text"`
	ParsedMapValue   string   `json:"parsed_map_value"`
	ParsedSeconds    int64    `json:"parsed_seconds"`
	ParsedNanos      int32    `json:"parsed_nanos"`
	Fields           []string `json:"fields"`
}

func main() {
	present := "present"
	msg := &compatibility.Compatibility{
		Present:    &present,
		Entries:    map[string]*compatibility.Compatibility_Nested{"key": {Value: "value"}},
		Payload:    &compatibility.Compatibility_Text{Text: "text"},
		ObservedAt: &timestamppb.Timestamp{Seconds: 123, Nanos: 456},
	}
	wire, err := msg.MarshalVT()
	if err != nil {
		panic(err)
	}
	input := append([]byte{}, wire...)
	if len(os.Args) > 1 {
		input, err = base64.StdEncoding.DecodeString(os.Args[1])
		if err != nil {
			panic(err)
		}
	}
	var parsed compatibility.Compatibility
	if err := parsed.UnmarshalVT(input); err != nil {
		panic(err)
	}
	roundTrip, err := parsed.MarshalVT()
	if err != nil {
		panic(err)
	}
	out, err := json.Marshal(result{
		Wire:             base64.StdEncoding.EncodeToString(wire),
		UnknownPreserved: bytes.Contains(roundTrip, []byte{0xa0, 0x06, 0x07}),
		ParsedPresent:    parsed.GetPresent(),
		ParsedText:       parsed.GetText(),
		ParsedMapValue:   parsed.GetEntries()["key"].GetValue(),
		ParsedSeconds:    parsed.GetObservedAt().GetSeconds(),
		ParsedNanos:      parsed.GetObservedAt().GetNanos(),
		Fields:           reflectedFields(),
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(string(out))
}

func reflectedFields() []string {
	fields := []string{}
	typ := reflect.TypeOf(compatibility.Compatibility{})
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		tag := field.Tag.Get("protobuf")
		if tag == "" {
			continue
		}
		fields = append(fields, formatTag(tag, field.Type))
	}
	for _, typ := range []reflect.Type{reflect.TypeOf(compatibility.Compatibility_Text{}), reflect.TypeOf(compatibility.Compatibility_Raw{})} {
		fields = append(fields, formatTag(typ.Field(0).Tag.Get("protobuf"), typ.Field(0).Type))
	}
	return fields
}

func formatTag(tag string, typ reflect.Type) string {
	parts := strings.Split(tag, ",")
	number := parts[1]
	name := ""
	for _, part := range parts[2:] {
		if strings.HasPrefix(part, "name=") {
			name = strings.TrimPrefix(part, "name=")
		}
	}
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	kind := "message"
	switch typ.Kind() {
	case reflect.String:
		kind = "string"
	case reflect.Slice:
		kind = "bytes"
	case reflect.Int32:
		kind = "enum"
	}
	suffix := ""
	if typ.Kind() == reflect.Map {
		suffix = ":map"
	} else if strings.Contains(typ.String(), "Timestamp") {
		suffix = ":timestamp"
	} else if strings.Contains(tag, ",oneof") {
		suffix = ":oneof"
	}
	return name + ":" + number + ":" + kind + suffix
}
