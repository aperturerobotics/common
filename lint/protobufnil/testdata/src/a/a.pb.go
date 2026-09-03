package a

type Inner struct {
	Value string `protobuf:"bytes,1,opt,name=value,proto3" json:"value,omitempty"`
}

func (x *Inner) Reset()                                          {}
func (x *Inner) String() string                                  { return "" }
func (x *Inner) SizeVT() int                                     { return 0 }
func (x *Inner) MarshalVT() ([]byte, error)                      { return nil, nil }
func (x *Inner) UnmarshalVT(b []byte) error                      { return nil }
func (x *Inner) MarshalToSizedBufferVT(dAtA []byte) (int, error) { return 0, nil }

type Outer struct {
	Inner *Inner `protobuf:"bytes,1,opt,name=inner,proto3" json:"inner,omitempty"`
}

func (x *Outer) Reset()                                          {}
func (x *Outer) String() string                                  { return "" }
func (x *Outer) SizeVT() int                                     { return 0 }
func (x *Outer) MarshalVT() ([]byte, error)                      { return nil, nil }
func (x *Outer) UnmarshalVT(b []byte) error                      { return nil }
func (x *Outer) MarshalToSizedBufferVT(dAtA []byte) (int, error) { return 0, nil }

func (x *Outer) GetInner() *Inner {
	if x != nil {
		return x.Inner
	}
	return nil
}
