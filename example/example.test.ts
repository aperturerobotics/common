import { describe, it, expect } from 'vitest'
import { ExampleMsg } from './example.pb.js'
import { OtherMsg } from './other/other.pb.js'

describe('ExampleMsg', () => {
  it('should create an empty message', () => {
    const msg = ExampleMsg.create()
    expect(msg).toEqual({})
  })

  it('should create a message with an example field', () => {
    const msg = ExampleMsg.create({ exampleField: 'hello' })
    expect(msg).toEqual({ exampleField: 'hello' })
  })

  it('should create a message with an other message field', () => {
    const other = OtherMsg.create({ fooField: 1}) 
    const msg = ExampleMsg.create({ otherMsg: other })
    expect(msg).toEqual({ otherMsg: { fooField: 1 } })
  })
})
