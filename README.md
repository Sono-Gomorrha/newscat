# newscat

![Logo](https://raw.github.com/slyrz/newscat/master/img/newscat_logo.png)

### Overview

newscat is a news content / article extractor written in just 1000
lines of Go code. It tries to deliver the news article content while
excluding clutter like image and video captions, editorial side notes,
"related content" article teasers, advertisements, user comments and meta
data.

### Getting Started

To download newscat, run

    go get github.com/slyrz/newscat

This will install newscat and, if not present, newscat's only non-standard
build dependency - the `html` package from the
[go.net networking libraries](http://code.google.com/p/go.net).

Then run

    go build github.com/slyrz/newscat

to build newscat. This should produce a `newscat` binary file in your
`$GOPATH/bin` directory.

### Usage

newscat accepts file paths and HTTP URLs as command line arguments.

    newscat [PATH]... [URL]...

If no arguments are passed, newscat expects HTML written to its
standard input.

    newscat < PATH

It prints the extracted article text to standard output. If you want
properly formatted paragraphs, pipe newscat's output to the `fmt` command.

    newscat ... | fmt

### Training and Evaluation

300 news articles were gathered by crawling top submissions from
various news-related Reddit communities.
A golden standard was created for every HTML page by manually adding a custom
HTML5 data attribute to the elements containing relevant content.

The data set was split into training and test data.
50 randomly chosen news articles were used to train newscat. The remaining
250 news articles were used to evaluate the rate the content
extraction quality.
For each news article, we compared the element-level predictions
with the real labels and calculated precision, recall and the balanced F-score.

![Extraction Quality](https://github.com/slyrz/newscat/raw/master/img/newscat_plot.png)

The above figure shows the resulting F-scores ordered in increasing magnitude.
The x-axis shows the percentage of articles whose F-scores fall below the
value indicated by the y-axis. In other words: the percentiles.

### License

newscat is released under MIT license.
You can find a copy of the MIT License in the [LICENSE](./LICENSE) file.
