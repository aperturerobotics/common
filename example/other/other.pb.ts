/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'example.other'

/** OtherMsg is a different message from ExampleMsg. */
export interface OtherMsg {
  /** FooField is an example field. */
  fooField: number
}

function createBaseOtherMsg(): OtherMsg {
  return { fooField: 0 }
}

export const OtherMsg = {
  encode(
    message: OtherMsg,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.fooField !== 0) {
      writer.uint32(8).uint32(message.fooField)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): OtherMsg {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOtherMsg()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.fooField = reader.uint32()
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OtherMsg, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<OtherMsg | OtherMsg[]>
      | Iterable<OtherMsg | OtherMsg[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [OtherMsg.encode(p).finish()]
        }
      } else {
        yield* [OtherMsg.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OtherMsg>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OtherMsg> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [OtherMsg.decode(p)]
        }
      } else {
        yield* [OtherMsg.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): OtherMsg {
    return { fooField: isSet(object.fooField) ? Number(object.fooField) : 0 }
  },

  toJSON(message: OtherMsg): unknown {
    const obj: any = {}
    if (message.fooField !== 0) {
      obj.fooField = Math.round(message.fooField)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OtherMsg>, I>>(base?: I): OtherMsg {
    return OtherMsg.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OtherMsg>, I>>(object: I): OtherMsg {
    const message = createBaseOtherMsg()
    message.fooField = object.fooField ?? 0
    return message
  },
}

type Builtin =
  | Date
  | Function
  | Uint8Array
  | string
  | number
  | boolean
  | undefined

export type DeepPartial<T> = T extends Builtin
  ? T
  : T extends Long
  ? string | number | Long
  : T extends Array<infer U>
  ? Array<DeepPartial<U>>
  : T extends ReadonlyArray<infer U>
  ? ReadonlyArray<DeepPartial<U>>
  : T extends { $case: string }
  ? { [K in keyof Omit<T, '$case'>]?: DeepPartial<T[K]> } & {
      $case: T['$case']
    }
  : T extends {}
  ? { [K in keyof T]?: DeepPartial<T[K]> }
  : Partial<T>

type KeysOfUnion<T> = T extends T ? keyof T : never
export type Exact<P, I extends P> = P extends Builtin
  ? P
  : P & { [K in keyof P]: Exact<P[K], I[K]> } & {
      [K in Exclude<keyof I, KeysOfUnion<P>>]: never
    }

if (_m0.util.Long !== Long) {
  _m0.util.Long = Long as any
  _m0.configure()
}

function isSet(value: any): boolean {
  return value !== null && value !== undefined
}
