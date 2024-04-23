// @generated by protoc-gen-es v1.9.0 with parameter "target=ts,ts_nocheck=false"
// @generated from file github.com/aperturerobotics/common/example/other/other.proto (package example.other, syntax proto3)
/* eslint-disable */

import type {
  BinaryReadOptions,
  FieldList,
  JsonReadOptions,
  JsonValue,
  PartialMessage,
  PlainMessage,
} from '@bufbuild/protobuf'
import { Message, proto3 } from '@bufbuild/protobuf'

/**
 * OtherMsg is a different message from ExampleMsg.
 *
 * @generated from message example.other.OtherMsg
 */
export class OtherMsg extends Message<OtherMsg> {
  /**
   * FooField is an example field.
   *
   * @generated from field: uint32 foo_field = 1;
   */
  fooField = 0

  constructor(data?: PartialMessage<OtherMsg>) {
    super()
    proto3.util.initPartial(data, this)
  }

  static readonly runtime: typeof proto3 = proto3
  static readonly typeName = 'example.other.OtherMsg'
  static readonly fields: FieldList = proto3.util.newFieldList(() => [
    { no: 1, name: 'foo_field', kind: 'scalar', T: 13 /* ScalarType.UINT32 */ },
  ])

  static fromBinary(
    bytes: Uint8Array,
    options?: Partial<BinaryReadOptions>,
  ): OtherMsg {
    return new OtherMsg().fromBinary(bytes, options)
  }

  static fromJson(
    jsonValue: JsonValue,
    options?: Partial<JsonReadOptions>,
  ): OtherMsg {
    return new OtherMsg().fromJson(jsonValue, options)
  }

  static fromJsonString(
    jsonString: string,
    options?: Partial<JsonReadOptions>,
  ): OtherMsg {
    return new OtherMsg().fromJsonString(jsonString, options)
  }

  static equals(
    a: OtherMsg | PlainMessage<OtherMsg> | undefined,
    b: OtherMsg | PlainMessage<OtherMsg> | undefined,
  ): boolean {
    return proto3.util.equals(OtherMsg, a, b)
  }
}
