This project delivers various functions to be used in your server, with that
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

