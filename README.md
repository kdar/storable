Shallow implementation of writing perl storable data.

Example
-------

    package main

    import (
      "fmt"
      "github.com/kdar/storable"
      "log"
    )

    type Demographics struct {
      Name string
      Age  int
    }

    func main() {
      data := &Demographics{Name: "kevin", Age: 123}
      b, err := storable.Marshal(data)
      if err != nil {
        log.Fatal(err)
      }

      fmt.Println(b)
    }
