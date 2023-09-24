/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'example'

/** ExampleMsg is an example message. */
export interface ExampleMsg {
  /** ExampleField is an example field. */
  exampleField: string
}

function createBaseExampleMsg(): ExampleMsg {
  return { exampleField: '' }
}

export const ExampleMsg = {
  encode(
    message: ExampleMsg,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.exampleField !== '') {
      writer.uint32(10).string(message.exampleField)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ExampleMsg {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseExampleMsg()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.exampleField = reader.string()
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
  // Transform<ExampleMsg, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ExampleMsg | ExampleMsg[]>
      | Iterable<ExampleMsg | ExampleMsg[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ExampleMsg.encode(p).finish()]
        }
      } else {
        yield* [ExampleMsg.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ExampleMsg>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ExampleMsg> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ExampleMsg.decode(p)]
        }
      } else {
        yield* [ExampleMsg.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): ExampleMsg {
    return {
      exampleField: isSet(object.exampleField)
        ? String(object.exampleField)
        : '',
    }
  },

  toJSON(message: ExampleMsg): unknown {
    const obj: any = {}
    if (message.exampleField !== '') {
      obj.exampleField = message.exampleField
    }
    return obj
  },

  create<I extends Exact<DeepPartial<ExampleMsg>, I>>(base?: I): ExampleMsg {
    return ExampleMsg.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<ExampleMsg>, I>>(
    object: I,
  ): ExampleMsg {
    const message = createBaseExampleMsg()
    message.exampleField = object.exampleField ?? ''
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
