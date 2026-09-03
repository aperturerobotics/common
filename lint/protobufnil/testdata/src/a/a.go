package a

func goodInit(outer *Outer) {
	// allowed: initialization pattern
	if outer.Inner == nil {
		outer.Inner = &Inner{}
	}
	_ = outer
}

func badPlainCheck(outer *Outer) {
	if outer.Inner == nil { // want `comparing a protobuf message pointer with nil`
		return
	}
}

func badGetterCheck(outer *Outer) bool {
	return outer.GetInner() != nil // want `comparing a protobuf message pointer with nil`
}

func goodNonProto(n *int) bool {
	return n == nil
}
