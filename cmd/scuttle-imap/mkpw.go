// +build ignore

package main

import (
	"fmt"
	"os"

	"github.com/cryptix/go/logging"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(os.Args[1]), bcrypt.DefaultCost)
	fmt.Println(fmt.Sprintf("%x", hashedPassword))
	logging.CheckFatal(err)
}
