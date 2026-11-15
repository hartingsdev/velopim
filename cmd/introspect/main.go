// Command introspect prints the fields of GraphQL types of the configured
// store's Admin API. Development aid; read only.
package main

import (
	"context"
	"fmt"
	"os"

	"velopim/internal/shopify"
)

type typeRef struct {
	Kind   string   `json:"kind"`
	Name   string   `json:"name"`
	OfType *typeRef `json:"ofType"`
}

func (t *typeRef) String() string {
	if t == nil {
		return ""
	}
	switch t.Kind {
	case "NON_NULL":
		return t.OfType.String() + "!"
	case "LIST":
		return "[" + t.OfType.String() + "]"
	}
	return t.Name
}

const ref = `kind name ofType { kind name ofType { kind name ofType { kind name ofType { kind name } } } }`

func main() {
	c, err := shopify.NewClient(shopify.Config{StoreDomain: os.Getenv("SHOPIFY_STORE_DOMAIN"),
		ClientID: os.Getenv("SHOPIFY_CLIENT_ID"), ClientSecret: os.Getenv("SHOPIFY_CLIENT_SECRET")})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	for _, name := range os.Args[1:] {
		var out struct {
			Type *struct {
				Kind        string
				InputFields []struct {
					Name string
					Type *typeRef
				}
				Fields []struct {
					Name string
					Type *typeRef
					Args []struct {
						Name string
						Type *typeRef
					}
				}
				EnumValues []struct{ Name string }
			} `json:"__type"`
		}
		q := `query($n: String!) { __type(name: $n) { kind inputFields { name type { ` + ref + ` } } fields { name type { ` + ref + ` } args { name type { ` + ref + ` } } } enumValues { name } } }`
		if err := c.Query(context.Background(), q, map[string]any{"n": name}, &out); err != nil {
			fmt.Println(name, err)
			continue
		}
		if out.Type == nil {
			fmt.Println(name, ": unknown type")
			continue
		}
		fmt.Printf("== %s (%s)\n", name, out.Type.Kind)
		for _, f := range out.Type.InputFields {
			fmt.Printf("  %s: %s\n", f.Name, f.Type)
		}
		for _, f := range out.Type.Fields {
			args := ""
			for _, a := range f.Args {
				args += a.Name + ": " + a.Type.String() + ", "
			}
			if args != "" {
				args = "(" + args[:len(args)-2] + ")"
			}
			fmt.Printf("  %s%s: %s\n", f.Name, args, f.Type)
		}
		for _, e := range out.Type.EnumValues {
			fmt.Printf("  %s\n", e.Name)
		}
	}
}
