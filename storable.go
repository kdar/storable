// Shallow implementation of writing perl storable data.
// Spec followed from: https://gitorious.org/python-storable/python-storable
//
// Notes:
// If I marshal a structure like this:
//      type NestedInner struct {
//        Name string
//      }
//
//      type NestedOuter struct {
//        nested NestedInner
//      }
// 
//      NestedOuter{
//        NestedInner{"Kevin"},
//      }
// 
// then perl's Data::Dumper returns:
//      $VAR1 = {
//        'Kevin' => undef,
//        'nested' => 'Name'
//      };
// 
// Same things happens if I use this perl hash:
//      my %ja = (
//        'nested' => (
//          'Name' => 'Kevin'
//        )
//      );
//
// I'm not entirely sure why. It only seems to work correctly
// if all nested structs are pointers (or in perl: nested refs). 
// Like the following:
//      NestedOuter{
//        NestedInner{"Kevin"},
//      }
// And the equivalent perl hash:
//      my %ja = (
//        'nested' => {
//          'Name' => 'Kevin'
//        }
//      );

package storable

import (
  "bytes"
  "encoding/binary"
  "io"
  "reflect"
  "strconv"
  "strings"
)

// perl storable spec:
// I use a number inside of the tags to indicate byte size
// <1:MAGIC> <1:VERSION>
// 
// Hash:
// <SX_HASH> <4:LEN> 
//   <TYPE ENTRY> <4:LEN OF KEY> <KEY> ...
//   <TYPE ENTRY> <4:LEN OF KEY> <KEY> ...
// 
// Array:
//   <SX_ARRAY> <4:LEN>
//     <TYPE ENTRY>
//     <TYPE ENTRY>...
//
// Scalar/Utf8str:
//   <1:TYPE> <1:LEN> <DATA>
//

const (
  MAGIC   = 0x5
  VERSION = 0x7

  SX_ARRAY   = 0x2  // ( 2): Array forthcoming (size, item list)
  SX_HASH    = 0x3  // ( 3): Hash forthcoming (size, key/value pair list)
  SX_REF     = 0x4  // ( 4): Reference to object forthcoming
  SX_UNDEF   = 0x5  // ( 5): Undefined scalar
  SX_SCALAR  = 0xa  // (10): Scalar (binary, small) follows (length, data)
  SX_UTF8STR = 0x17 // (23): UTF-8 string forthcoming (small)
)

type Encoder struct {
  w   io.Writer
  e   encodeState
  err error
}

func NewEncoder(w io.Writer) *Encoder {
  return &Encoder{w: w}
}

func (enc *Encoder) Encode(v interface{}) error {
  enc.e.Reset()

  err := enc.e.marshal(v)
  if _, err = enc.w.Write(enc.e.Bytes()); err != nil {
    enc.err = err
  }

  return err
}

func Marshal(v interface{}) ([]byte, error) {
  e := &encodeState{}
  err := e.marshal(v)

  return e.Bytes(), err
}

// An encodeState encodes storable into a bytes.Buffer.
type encodeState struct {
  bytes.Buffer // accumulated output
}

func (e *encodeState) marshal(v interface{}) (err error) {
  err = binary.Write(e, binary.BigEndian, uint8(MAGIC))
  if err != nil {
    return err
  }

  err = binary.Write(e, binary.BigEndian, uint8(VERSION))
  if err != nil {
    return err
  }

  // d := reflect.ValueOf(v)
  // if d.Kind() == reflect.Ptr {
  //   err = e.marshalValue(d.Elem())
  // } else {
  //   err = e.marshalValue(d)
  // }

  err = e.marshalValue(reflect.ValueOf(v))

  return err
}

func (e *encodeState) marshalValue(value reflect.Value) error {
  var err error

  if value.Kind() == reflect.Ptr {
    value = value.Elem()

    err = binary.Write(e, binary.BigEndian, uint8(SX_REF))
    if err != nil {
      return err
    }
  }

  typ := value.Type()
  switch typ.Kind() {
  case reflect.Struct:
    err = e.marshalStruct(value)
  case reflect.Slice, reflect.Array:
    err = e.marshalSlice(value)
  case reflect.Bool:
    err = e.marshalBool(value)
  case reflect.String:
    err = e.marshalString(value)
  case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
    err = e.marshalInt(value)
  case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
    err = e.marshalUint(value)
  case reflect.Float32, reflect.Float64:
    err = e.marshalFloat(value)
  }

  return err
}

func (e *encodeState) marshalStruct(value reflect.Value) error {
  err := binary.Write(e, binary.BigEndian, uint8(SX_HASH))
  if err != nil {
    return err
  }

  typ := value.Type()
  n := typ.NumField()

  totalSize := 0
  // write serialize children in temporary buffer since
  // we find out how many children there are later and we
  // need to write the children size first.
  e2 := &encodeState{}
  for i := 0; i < n; i++ {
    f := typ.Field(i)
    fieldValue := value.FieldByName(f.Name)

    fopts := strings.Split(f.Tag.Get("storable"), ",")
    if len(fopts) > 0 && fopts[0] == "omitempty" && fieldValue.Len() == 0 {
      continue
    }
    totalSize++

    err = e2.marshalValue(fieldValue)
    if err != nil {
      return err
    }

    // Write hash key
    binary.Write(e2, binary.BigEndian, uint32(len(f.Name)))
    _, err = e2.WriteString(f.Name)
    if err != nil {
      return err
    }
  }

  err = binary.Write(e, binary.BigEndian, uint32(totalSize))
  if err != nil {
    return err
  }

  _, err = e.Write(e2.Bytes())

  return err
}

func (e *encodeState) marshalSlice(value reflect.Value) error {
  var err error

  err = binary.Write(e, binary.BigEndian, uint8(SX_ARRAY))
  if err != nil {
    return err
  }

  n := value.Len()
  err = binary.Write(e, binary.BigEndian, uint32(n))
  if err != nil {
    return err
  }

  for i := 0; i < n; i++ {
    err = e.marshalValue(value.Index(i))
    if err != nil {
      return err
    }
  }

  return nil
}

func (e *encodeState) marshalBool(value reflect.Value) error {
  var err error

  err = binary.Write(e, binary.BigEndian, uint8(SX_SCALAR))
  if err != nil {
    return err
  }
  err = binary.Write(e, binary.BigEndian, uint8(1))
  if err != nil {
    return err
  }

  if value.Bool() {
    _, err = e.Write([]byte(strconv.FormatInt(1, 10)))
  } else {
    _, err = e.Write([]byte(strconv.FormatInt(0, 10)))
  }

  return err
}

func (e *encodeState) marshalString(value reflect.Value) error {
  var err error

  err = binary.Write(e, binary.BigEndian, uint8(SX_SCALAR))
  if err != nil {
    return err
  }
  err = binary.Write(e, binary.BigEndian, uint8(value.Len()))
  if err != nil {
    return err
  }

  //err = binary.Write(e, binary.BigEndian, uint8(SX_UTF8STR))
  //err = binary.Write(e, binary.BigEndian, uint8(value.Len()))

  _, err = e.Write([]byte(value.String()))
  return err
}

func (e *encodeState) marshalInt(value reflect.Value) error {
  var err error

  err = binary.Write(e, binary.BigEndian, uint8(SX_SCALAR))
  if err != nil {
    return err
  }

  s := strconv.FormatInt(value.Int(), 10)
  err = binary.Write(e, binary.BigEndian, uint8(len(s)))
  if err != nil {
    return err
  }

  _, err = e.Write([]byte(s))
  return err
}

func (e *encodeState) marshalUint(value reflect.Value) error {
  var err error

  err = binary.Write(e, binary.BigEndian, uint8(SX_SCALAR))
  if err != nil {
    return err
  }

  s := strconv.FormatUint(value.Uint(), 10)
  err = binary.Write(e, binary.BigEndian, uint8(len(s)))
  if err != nil {
    return err
  }

  _, err = e.Write([]byte(s))
  return err
}

func (e *encodeState) marshalFloat(value reflect.Value) error {
  var err error

  err = binary.Write(e, binary.BigEndian, uint8(SX_SCALAR))
  if err != nil {
    return err
  }

  s := strconv.FormatFloat(value.Float(), 'g', -1, value.Type().Bits())
  err = binary.Write(e, binary.BigEndian, uint8(len(s)))
  if err != nil {
    return err
  }

  _, err = e.Write([]byte(s))
  return err
}
