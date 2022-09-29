This project delivers various handlers to be used in your server, with that
you can customize all pages and paths.

# Usage

The simplest way of using this project is the following example:

```
package main

import (
	"net/http"

	"blmayer.dev/gwi"
)

func main() {
	g, _ := gwi.NewGwi("templates", "git")
	// handle error

	err := http.ListenAndServe(":8080", g.Handle())
	// handle err
}
```

This will use all default handlers, giving you an index page with a list
of git repositories in folder *git*.

Each handler can be specified as well:

```
func main() {
	...

	http.Handle("/git", g.IndexHandler)
}
```

This will register the index page in */git* instead of */*.
