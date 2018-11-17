# testdiff

[![Build Status](https://travis-ci.org/catherinejones/testdiff.svg?branch=master)](https://travis-ci.org/catherinejones/testdiff)
[![Test Coverage](https://api.codeclimate.com/v1/badges/b2656dddadb2b4665733/test_coverage)](https://codeclimate.com/github/catherinejones/testdiff/test_coverage)
[![Go Report Card](https://goreportcard.com/badge/github.com/catherinejones/testdiff)](https://goreportcard.com/report/github.com/catherinejones/testdiff)

testdiff is a simple library specifically for testing purposes, which adds a very simple function for comparing two strings. 

## Installing testdiff

Install testdiff by calling:

```
go get -u github.com/catherinejones/testdiff
```

## Using testdiff

Import testdiff by calling

```golang
import "github.com/catherinejones/testdiff"
```

testdiff's main functions are:

1. `StringIs`
2. `StringIsNot`
3. `StringIsFile`
4. `StringIsNotFile`

`StringIs` and `StringIsNot` test if a string is a specific value. `StringIs` returns a diff if the two passed strings are not equal: 

```golang
import (
    "testing"
    "github.com/catherinejones/testdiff"
)

func TestExample(t *testing.T) {
    testdiff.StringIs(t, exampleString, exampleString)
    testdiff.StringIsNot(t, exampleString, exampleDiffString)
}
```

Both of the tests in `TestExample` should pass.

```golang
import (
    "testing"
    "github.com/catherinejones/testdiff"
)

func TestBadExample(t *testing.T) {
    testdiff.StringIs(t, exampleString, exampleDiffString)
}
```

This example would return an error message with the diff in the test output.

![The error message showing that there are a few character differences between the two strings](images/example.png)

`StringIsFile` and `StringIsNotFile` test if a string is equal to the contents of the passed filename. `StringIsFile`, like `StringIs`, will return a diff if the string doesn't match the contents of the file at the passed filename:

```golang
import (
    "testing"
    "github.com/catherinejones/testdiff"
)

func TestFileExample(t *testing.T) {
    // "exampleString.txt" contains the contents of exampleString.
    testdiff.StringIsFile(t, "exampleString.txt", exampleString)
    testdiff.StringIsNotFile(t, "exampleString.txt", exampleDiffString)
}
```

Both of the tests in `TestFileExample` should pass.

The low-level way of interacting with testdiff is to use the `CompareStrings` function. It takes two strings as input, and returns true or false depending on if they're equal or not, respectively. It also returns an `error`, with a listing of the lines that differ between the two strings.

```golang
import "github.com/catherinejones/testdiff"

func Example() {
    compare, err = testdiff.CompareStrings(exampleString, exampleDiffString)
    if !compare {
        fmt.Println("Error: ", err)
    }
}
```

