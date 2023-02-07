GWI stands for Git Web Interface, that is it delivers a ready to use
visualization and management tool atop of your git repositories.

With GWI you can easily host your own git platform customized
to your needs. Some features are:

- Easy to setup
- Pages are templates you can customize
- Does not depend on git or CGI scripts
- Lightweight
- Free
- Under active development

This project is in early stages of development, and some features may be
missing. If you want to request a feature or report a bug, follow the
instructions at [the author's git](https://blmayer.dev/x).

For more information on how to use gwi, please consult our 
[go docs](https://pkg.go.dev/blmayer.dev/x/gwi).

Thank you!


# Usage

The simplest way of using this project is the following example:

```
package main

import (
	"net/http"

	"blmayer.dev/gwi"
)

func main() {
	// init user vault
	v, err := NewFileVault("users.json", "--salt--")
	// handle error
	
	// gwi config struct
	c := gwi.Config{
		Root: "path/to/git/folder",
		PagesRoot: "path/to/html-templates",
		...
	}

	g, _ := gwi.NewFromConfig(c, v)
	// handle error

	err := http.ListenAndServe(":8080", g.Handle())
	// handle err
}
```

## Examples


### Users

To get a list of your users is simple:

```
<ul>
	{{range users}}
	<li>{{.}}</li>
	{{end}}
</ul>
```


### File tree

To get the file tree for the current reference:

```
<table>
    <tr>
        <th>Mode</th>
        <th>Size</th>
        <th>Name</th>
    </tr>
    {{range tree .Ref}}
    <tr>
        <td>{{.Mode}}</td>
        <td>{{.Size}}</td>
	<td>{{.Name}}</td>
    </tr>
    {{end}}
</table>
```

Will print a nice list of your project files.


### Commits

Using the functions `commits` and `commit` you're able to see a list of
commits and check details of each one:

```
<table>
    <tr>
        <th>Time</th>
        <th>Author</th>
        <th>Message</th>
    </tr>
    {{range commits .Ref}}
    <tr>
        <td>{{.Author.When.String}}</td>
        <td>{{.Author.Name}}</td>
	<td>{{.Message}}</td>
    </tr>
    {{end}}
</table>
```

To get the list, and the following show a commit's details:

```
{{with commit .Ref}}

<p><b>Commited at:</b> {{.Committer.When.String}}</p>
<p><b>Author:</b> {{.Committer.Name}} ({{.Committer.Email}})</p>
<p><b>Message:</b></p>
<p>{{.Message}}</p>
{{end}}
```

