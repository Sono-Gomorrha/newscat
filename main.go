package main

import (
	"fmt"
	"github.com/slyrz/newscat/html"
	"github.com/slyrz/newscat/model"
	"os"
)

func printContent(doc *html.Document, clf *model.Classifier) {
	var last *html.Chunk = nil
	var delim string = ""

	for i, feature := range model.Features(doc) {
		if !clf.Predict(&feature) {
			continue
		}
		switch {
		case last == nil:
			delim = ""
		case last.Block != doc.Chunks[i].Block:
			delim = "\n\n"
		case last.Block == doc.Chunks[i].Block:
			delim = " "
		}
		fmt.Printf("%s%s", delim, doc.Chunks[i].Text)
		last = doc.Chunks[i]
	}
	fmt.Println()
}

func main() {
	clf := model.NewClassifier()
	for _, arg := range os.Args[1:] {
		file, err := os.Open(arg)
		if err != nil {
			panic(err)
		}
		defer file.Close()

		doc, err := html.NewDocument(file)
		if err != nil {
			panic(err)
		}
		printContent(doc, clf)
	}
}
