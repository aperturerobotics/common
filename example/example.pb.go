// Code generated by protoc-gen-go-lite. DO NOT EDIT.
// protoc-gen-go-lite version: v0.6.0
// source: github.com/aperturerobotics/common/example/example.proto

package example

import (
	fmt "fmt"
	io "io"
	strconv "strconv"
	strings "strings"

	other "github.com/aperturerobotics/common/example/other"
	protobuf_go_lite "github.com/aperturerobotics/protobuf-go-lite"
	json "github.com/aperturerobotics/protobuf-go-lite/json"
)

// ExampleMsg is an example message.
type ExampleMsg struct {
	unknownFields []byte
	// ExampleField is an example field.
	ExampleField string `protobuf:"bytes,1,opt,name=example_field,json=exampleField,proto3" json:"exampleField,omitempty"`
	// OtherMsg is an example of an imported message field.
	OtherMsg *other.OtherMsg `protobuf:"bytes,2,opt,name=other_msg,json=otherMsg,proto3" json:"otherMsg,omitempty"`
}

func (x *ExampleMsg) Reset() {
	*x = ExampleMsg{}
}

func (*ExampleMsg) ProtoMessage() {}

func (x *ExampleMsg) GetExampleField() string {
	if x != nil {
		return x.ExampleField
	}
	return ""
}

func (x *ExampleMsg) GetOtherMsg() *other.OtherMsg {
	if x != nil {
		return x.OtherMsg
	}
	return nil
}

func (m *ExampleMsg) CloneVT() *ExampleMsg {
	if m == nil {
		return (*ExampleMsg)(nil)
	}
	r := new(ExampleMsg)
	r.ExampleField = m.ExampleField
	if rhs := m.OtherMsg; rhs != nil {
		r.OtherMsg = rhs.CloneVT()
	}
	if len(m.unknownFields) > 0 {
		r.unknownFields = make([]byte, len(m.unknownFields))
		copy(r.unknownFields, m.unknownFields)
	}
	return r
}

func (m *ExampleMsg) CloneMessageVT() protobuf_go_lite.CloneMessage {
	return m.CloneVT()
}

func (this *ExampleMsg) EqualVT(that *ExampleMsg) bool {
	if this == that {
		return true
	} else if this == nil || that == nil {
		return false
	}
	if this.ExampleField != that.ExampleField {
		return false
	}
	if !this.OtherMsg.EqualVT(that.OtherMsg) {
		return false
	}
	return string(this.unknownFields) == string(that.unknownFields)
}

func (this *ExampleMsg) EqualMessageVT(thatMsg any) bool {
	that, ok := thatMsg.(*ExampleMsg)
	if !ok {
		return false
	}
	return this.EqualVT(that)
}

// MarshalProtoJSON marshals the ExampleMsg message to JSON.
func (x *ExampleMsg) MarshalProtoJSON(s *json.MarshalState) {
	if x == nil {
		s.WriteNil()
		return
	}
	s.WriteObjectStart()
	var wroteField bool
	if x.ExampleField != "" || s.HasField("exampleField") {
		s.WriteMoreIf(&wroteField)
		s.WriteObjectField("exampleField")
		s.WriteString(x.ExampleField)
	}
	if x.OtherMsg != nil || s.HasField("otherMsg") {
		s.WriteMoreIf(&wroteField)
		s.WriteObjectField("otherMsg")
		x.OtherMsg.MarshalProtoJSON(s.WithField("otherMsg"))
	}
	s.WriteObjectEnd()
}

// MarshalJSON marshals the ExampleMsg to JSON.
func (x *ExampleMsg) MarshalJSON() ([]byte, error) {
	return json.DefaultMarshalerConfig.Marshal(x)
}

// UnmarshalProtoJSON unmarshals the ExampleMsg message from JSON.
func (x *ExampleMsg) UnmarshalProtoJSON(s *json.UnmarshalState) {
	if s.ReadNil() {
		return
	}
	s.ReadObject(func(key string) {
		switch key {
		default:
			s.Skip() // ignore unknown field
		case "example_field", "exampleField":
			s.AddField("example_field")
			x.ExampleField = s.ReadString()
		case "other_msg", "otherMsg":
			if s.ReadNil() {
				x.OtherMsg = nil
				return
			}
			x.OtherMsg = &other.OtherMsg{}
			x.OtherMsg.UnmarshalProtoJSON(s.WithField("other_msg", true))
		}
	})
}

// UnmarshalJSON unmarshals the ExampleMsg from JSON.
func (x *ExampleMsg) UnmarshalJSON(b []byte) error {
	return json.DefaultUnmarshalerConfig.Unmarshal(b, x)
}

func (m *ExampleMsg) MarshalVT() (dAtA []byte, err error) {
	if m == nil {
		return nil, nil
	}
	size := m.SizeVT()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBufferVT(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *ExampleMsg) MarshalToVT(dAtA []byte) (int, error) {
	size := m.SizeVT()
	return m.MarshalToSizedBufferVT(dAtA[:size])
}

func (m *ExampleMsg) MarshalToSizedBufferVT(dAtA []byte) (int, error) {
	if m == nil {
		return 0, nil
	}
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if m.unknownFields != nil {
		i -= len(m.unknownFields)
		copy(dAtA[i:], m.unknownFields)
	}
	if m.OtherMsg != nil {
		size, err := m.OtherMsg.MarshalToSizedBufferVT(dAtA[:i])
		if err != nil {
			return 0, err
		}
		i -= size
		i = protobuf_go_lite.EncodeVarint(dAtA, i, uint64(size))
		i--
		dAtA[i] = 0x12
	}
	if len(m.ExampleField) > 0 {
		i -= len(m.ExampleField)
		copy(dAtA[i:], m.ExampleField)
		i = protobuf_go_lite.EncodeVarint(dAtA, i, uint64(len(m.ExampleField)))
		i--
		dAtA[i] = 0xa
	}
	return len(dAtA) - i, nil
}

func (m *ExampleMsg) SizeVT() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	l = len(m.ExampleField)
	if l > 0 {
		n += 1 + l + protobuf_go_lite.SizeOfVarint(uint64(l))
	}
	if m.OtherMsg != nil {
		l = m.OtherMsg.SizeVT()
		n += 1 + l + protobuf_go_lite.SizeOfVarint(uint64(l))
	}
	n += len(m.unknownFields)
	return n
}

func (x *ExampleMsg) MarshalProtoText() string {
	var sb strings.Builder
	sb.WriteString("ExampleMsg { ")
	if x.ExampleField != "" {
		sb.WriteString(" example_field: ")
		sb.WriteString(strconv.Quote(x.ExampleField))
	}
	if x.OtherMsg != nil {
		sb.WriteString(" other_msg: ")
		sb.WriteString(x.OtherMsg.MarshalProtoText())
	}
	sb.WriteString("}")
	return sb.String()
}
func (x *ExampleMsg) String() string {
	return x.MarshalProtoText()
}
func (m *ExampleMsg) UnmarshalVT(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return protobuf_go_lite.ErrIntOverflow
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: ExampleMsg: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: ExampleMsg: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field ExampleField", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return protobuf_go_lite.ErrIntOverflow
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return protobuf_go_lite.ErrInvalidLength
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return protobuf_go_lite.ErrInvalidLength
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.ExampleField = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field OtherMsg", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return protobuf_go_lite.ErrIntOverflow
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return protobuf_go_lite.ErrInvalidLength
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return protobuf_go_lite.ErrInvalidLength
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if m.OtherMsg == nil {
				m.OtherMsg = &other.OtherMsg{}
			}
			if err := m.OtherMsg.UnmarshalVT(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := protobuf_go_lite.Skip(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if (skippy < 0) || (iNdEx+skippy) < 0 {
				return protobuf_go_lite.ErrInvalidLength
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			m.unknownFields = append(m.unknownFields, dAtA[iNdEx:iNdEx+skippy]...)
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
