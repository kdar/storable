package storable

import (
  "bytes"
  "os/exec"
  "testing"
)

type MarshalTest struct {
  in  interface{}
  out string
}

var thawCode = `use Storable; 
use Data::Dumper; 
$Data::Dumper::Indent = 0;
$Data::Dumper::Terse = 1;
@in = <STDIN>; 
print Dumper(Storable::thaw(join('', @in)));`

type nested struct {
  Name string
}

var (
  marshalTests = []MarshalTest{
    {struct{ Name string }{"Kevin"}, `{'Name' => 'Kevin'}`},

    {struct {
      Name string
      Omit string `storable:"omitempty"`
    }{Name: "Kevin"}, `{'Name' => 'Kevin'}`},

    {struct {
      nested *nested
    }{&nested{"Kevin"}}, `{'nested' => {'Name' => 'Kevin'}}`},

    {"Kevin", `\'Kevin'`},
    {1234, `\'1234'`},
    {5.55, `\'5.55'`},
    {false, `\'0'`},
    {[]string{"hey", "there"}, `['hey','there']`},
  }
)

func TestMarshal(t *testing.T) {
  for i, tt := range marshalTests {
    cmd := exec.Command("perl", "-e", thawCode)

    b, err := Marshal(tt.in)
    if err != nil {
      t.Fatal(err)
    }
    cmd.Stdin = bytes.NewBuffer(b)

    out, err := cmd.CombinedOutput()
    if err != nil {
      t.Fatal(err)
    }

    if tt.out != string(out) {
      t.Fatalf("%d. unexpected output: thaw(%#v) => %q. want %q", i, tt.in, string(out), tt.out)
    }
  }
}
