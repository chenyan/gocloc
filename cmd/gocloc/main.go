package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/hhatto/gocloc"
	"github.com/jessevdk/go-flags"
	"golang.org/x/term"
)

// Version is version string for gocloc command
var Version string

// GitCommit is git commit hash string for gocloc command
var GitCommit string

// OutputTypeDefault is cloc's text output format for --output-type option
const OutputTypeDefault string = "default"

// OutputTypeClocXML is Cloc's XML output format for --output-type option
const OutputTypeClocXML string = "cloc-xml"

// OutputTypeSloccount is Sloccount output format for --output-type option
const OutputTypeSloccount string = "sloccount"

// OutputTypeJSON is JSON output format for --output-type option
const OutputTypeJSON string = "json"

// OutputTypeMarkdown is Markdown output format for --output-type option
const OutputTypeMarkdown string = "markdown"

const (
	ansiBrightYellow = "\x1b[93m"
	ansiReset        = "\x1b[0m"
)

const (
	fileHeader             string = "File"
	languageHeader         string = "Language"
	commonHeader           string = "files          blank        comment           code"
	defaultOutputSeparator string = "-------------------------------------------------------------------------" +
		"-------------------------------------------------------------------------" +
		"-------------------------------------------------------------------------"
)

var rowLen = 79

// CmdOptions is gocloc command options.
// It is necessary to use notation that follows go-flags.
type CmdOptions struct {
	ByFile         bool   `long:"by-file" description:"report results for every encountered source file"`
	SortTag        string `long:"sort" default:"code" description:"sort based on a certain column" choice:"name" choice:"files" choice:"blank" choice:"comment" choice:"code"`
	OutputType     string `long:"output-type" default:"default" description:"output type [values: default,markdown,cloc-xml,sloccount,json]"`
	ExcludeExt     string `long:"exclude-ext" description:"exclude file name extensions (separated commas)"`
	IncludeLang    string `long:"include-lang" description:"include language name (separated commas)"`
	Match          string `long:"match" description:"include file name (regex)"`
	NotMatch       string `long:"not-match" description:"exclude file name (regex)"`
	MatchDir       string `long:"match-d" description:"include dir name (regex)"`
	NotMatchDir    string `long:"not-match-d" description:"exclude dir name (regex)"`
	Fullpath       bool   `long:"fullpath" description:"apply match/not-match options to full file paths instead of base names"`
	Debug          bool   `long:"debug" description:"dump debug log for developer"`
	SkipDuplicated bool   `long:"skip-duplicated" description:"skip duplicated files"`
	ShowLang       bool   `long:"show-lang" description:"print about all languages and extensions"`
	ShowVersion    bool   `long:"version" description:"print version info"`
	Depth          int    `long:"depth" short:"d" default:"0" description:"show per-directory statistics up to this many levels below each PATH (0=disabled); implies language summary per folder, indented as a tree"`
}

type outputBuilder struct {
	opts   *CmdOptions
	result *gocloc.Result
}

func newOutputBuilder(result *gocloc.Result, opts *CmdOptions) *outputBuilder {
	return &outputBuilder{
		opts,
		result,
	}
}

func (o *outputBuilder) WriteHeader() {
	maxPathLen := o.result.MaxPathLength
	headerLen := 28
	header := languageHeader

	if o.opts.ByFile {
		headerLen = maxPathLen + 1
		rowLen = maxPathLen + len(commonHeader) + 2
		header = fileHeader
	}

	if o.opts.OutputType == OutputTypeDefault {
		fmt.Printf("%.[2]*[1]s\n", defaultOutputSeparator, rowLen)
		fmt.Printf("%-[2]*[1]s %[3]s\n", header, headerLen, commonHeader)
		fmt.Printf("%.[2]*[1]s\n", defaultOutputSeparator, rowLen)
	}

	if o.opts.OutputType == OutputTypeMarkdown {
		allHeaders := fmt.Sprintf("%s%s%s", header, strings.Repeat(" ", headerLen), commonHeader)
		headerString := "| " + gocloc.InsertPipesInTheMiddle(allHeaders)
		fmt.Println(headerString)

		for i := 0; i < len(headerString); i++ {
			if headerString[i] == '|' {
				fmt.Print("|")
			} else {
				if i == 1 {
					// Align the first column to the left
					fmt.Print(":")
				} else {
					// Align the other columns to the right
					if headerString[i+1] == '|' && i > headerLen {
						fmt.Print(":")
					} else {
						fmt.Print("-")
					}
				}
			}
		}

		fmt.Println()
	}
}

func (o *outputBuilder) WriteFooter() {
	total := o.result.Total
	maxPathLen := o.result.MaxPathLength

	if o.opts.OutputType == OutputTypeDefault {
		fmt.Printf("%.[2]*[1]s\n", defaultOutputSeparator, rowLen)
		if o.opts.ByFile {
			fmt.Printf("%-[1]*[2]v %6[3]v %14[4]v %14[5]v %14[6]v\n",
				maxPathLen, "TOTAL", total.Total, total.Blanks, total.Comments, total.Code)
		} else {
			fmt.Printf("%-27v %6v %14v %14v %14v\n",
				"TOTAL", total.Total, total.Blanks, total.Comments, total.Code)
		}
		fmt.Printf("%.[2]*[1]s\n", defaultOutputSeparator, rowLen)
	}

	if o.opts.OutputType == OutputTypeMarkdown {
		if o.opts.ByFile {
			fmt.Printf("| %-[1]*[2]v |%10v|%12v|%14v|%8v |\n", maxPathLen, "", "", "", "", "")
			fmt.Printf("| %-[1]*[2]v |%9v |%11v |%13v |%8v |\n", maxPathLen, "TOTAL", total.Total, total.Blanks, total.Comments, total.Code)
		} else {
			fmt.Printf("| %21v|%22v|%12v|%14v|%8v |\n", "", "", "", "", "")
			fmt.Printf("| %20v |%21v |%11v |%13v |%8v |\n", "TOTAL", total.Total, total.Blanks, total.Comments, total.Code)
		}
	}
}

func writeResultWithByFile(opts *CmdOptions, result *gocloc.Result) {
	clocFiles := result.Files
	total := result.Total
	maxPathLen := result.MaxPathLength

	var sortedFiles gocloc.ClocFiles
	for _, file := range clocFiles {
		sortedFiles = append(sortedFiles, *file)
	}
	switch opts.SortTag {
	case "name":
		sortedFiles.SortByName()
	case "comment":
		sortedFiles.SortByComments()
	case "blank":
		sortedFiles.SortByBlanks()
	default:
		sortedFiles.SortByCode()
	}

	switch opts.OutputType {
	case OutputTypeClocXML:
		t := gocloc.XMLTotalFiles{
			Code:    total.Code,
			Comment: total.Comments,
			Blank:   total.Blanks,
		}
		f := &gocloc.XMLResultFiles{
			Files: sortedFiles,
			Total: t,
		}
		xmlResult := gocloc.XMLResult{
			XMLFiles: f,
		}
		xmlResult.Encode()
	case OutputTypeSloccount:
		for _, file := range sortedFiles {
			p := ""
			if strings.HasPrefix(file.Name, "./") || string(file.Name[0]) == "/" {
				splitPaths := strings.Split(file.Name, string(os.PathSeparator))
				if len(splitPaths) >= 3 {
					p = splitPaths[1]
				}
			}
			fmt.Printf("%v\t%v\t%v\t%v\n",
				file.Code, file.Lang, p, file.Name)
		}
	case OutputTypeJSON:
		jsonResult := gocloc.NewJSONFilesResultFromCloc(total, sortedFiles)
		buf, err := json.Marshal(jsonResult)
		if err != nil {
			fmt.Println(err)
			panic("json marshal error")
		}
		os.Stdout.Write(buf)
	case OutputTypeMarkdown:
		for _, file := range sortedFiles {
			clocFile := file
			fmt.Printf("| %-[1]*[2]s |%8[3]v  |%11[4]v |%13[5]v |%8[6]v |\n",
				maxPathLen, file.Name, 1, clocFile.Blanks, clocFile.Comments, clocFile.Code)
		}

	default:
		for _, file := range sortedFiles {
			clocFile := file
			fmt.Printf("%-[1]*[2]s %21[3]v %14[4]v %14[5]v\n",
				maxPathLen, file.Name, clocFile.Blanks, clocFile.Comments, clocFile.Code)
		}
	}
}

func (o *outputBuilder) WriteResult() {
	// write header
	o.WriteHeader()

	clocLangs := o.result.Languages
	total := o.result.Total

	if o.opts.ByFile {
		writeResultWithByFile(o.opts, o.result)
	} else {
		var sortedLanguages gocloc.Languages
		for _, language := range clocLangs {
			if len(language.Files) != 0 {
				sortedLanguages = append(sortedLanguages, *language)
			}
		}
		switch o.opts.SortTag {
		case "name":
			sortedLanguages.SortByName()
		case "files":
			sortedLanguages.SortByFiles()
		case "comment":
			sortedLanguages.SortByComments()
		case "blank":
			sortedLanguages.SortByBlanks()
		default:
			sortedLanguages.SortByCode()
		}

		switch o.opts.OutputType {
		case OutputTypeClocXML:
			xmlResult := gocloc.NewXMLResultFromCloc(total, sortedLanguages, gocloc.XMLResultWithLangs)
			xmlResult.Encode()
		case OutputTypeJSON:
			jsonResult := gocloc.NewJSONLanguagesResultFromCloc(total, sortedLanguages)
			buf, err := json.Marshal(jsonResult)
			if err != nil {
				fmt.Println(err)
				panic("json marshal error")
			}
			os.Stdout.Write(buf)
		case OutputTypeMarkdown:
			for _, language := range sortedLanguages {
				fmt.Printf("| %-20v |%21v |%11v |%13v |%8v |\n",
					language.Name, len(language.Files), language.Blanks, language.Comments, language.Code)
			}
		default:
			for _, language := range sortedLanguages {
				fmt.Printf("%-27v %6v %14v %14v %14v\n",
					language.Name, len(language.Files), language.Blanks, language.Comments, language.Code)
			}
		}
	}

	// write footer
	o.WriteFooter()
}

// langAgg holds aggregated counts for one language under one directory.
type langAgg struct {
	files    int32
	blanks   int32
	comments int32
	code     int32
}

type langRow struct {
	name     string
	files    int32
	blanks   int32
	comments int32
	code     int32
}

func absPathRoots(paths []string) ([]string, error) {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		a, err := filepath.Abs(p)
		if err != nil {
			return nil, err
		}
		out = append(out, filepath.Clean(a))
	}
	return out, nil
}

func rootForPath(fileAbs string, roots []string) (string, bool) {
	var best string
	var bestLen int
	for _, r := range roots {
		if fileAbs == r || strings.HasPrefix(fileAbs, r+string(os.PathSeparator)) {
			if len(r) > bestLen {
				best = r
				bestLen = len(r)
			}
		}
	}
	return best, best != ""
}

func splitDirParts(dirRel string) []string {
	if dirRel == "." || dirRel == "" {
		return nil
	}
	parts := strings.Split(dirRel, string(os.PathSeparator))
	var clean []string
	for _, p := range parts {
		if p != "" && p != "." {
			clean = append(clean, p)
		}
	}
	return clean
}

func aggregateByDepth(result *gocloc.Result, absRoots []string, depth int) map[string]map[string]*langAgg {
	dirs := make(map[string]map[string]*langAgg)

	add := func(dirKey, lang string, cf *gocloc.ClocFile) {
		m := dirs[dirKey]
		if m == nil {
			m = make(map[string]*langAgg)
			dirs[dirKey] = m
		}
		la := m[lang]
		if la == nil {
			la = &langAgg{}
			m[lang] = la
		}
		la.files++
		la.blanks += cf.Blanks
		la.comments += cf.Comments
		la.code += cf.Code
	}

	for _, cf := range result.Files {
		fileAbs, err := filepath.Abs(cf.Name)
		if err != nil {
			continue
		}
		fileAbs = filepath.Clean(fileAbs)
		root, ok := rootForPath(fileAbs, absRoots)
		if !ok {
			continue
		}
		rel, err := filepath.Rel(root, fileAbs)
		if err != nil {
			continue
		}
		add(root, cf.Lang, cf)
		dirRel := filepath.Dir(rel)
		parts := splitDirParts(dirRel)
		for i := 1; i <= depth && i <= len(parts); i++ {
			sub := filepath.Join(append([]string{root}, parts[0:i]...)...)
			add(sub, cf.Lang, cf)
		}
	}
	return dirs
}

func sortLangRows(rows []langRow, sortTag string) {
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		switch sortTag {
		case "name":
			return a.name < b.name
		case "files":
			if a.files == b.files {
				return a.code > b.code
			}
			return a.files > b.files
		case "comment":
			if a.comments == b.comments {
				return a.code > b.code
			}
			return a.comments > b.comments
		case "blank":
			if a.blanks == b.blanks {
				return a.code > b.code
			}
			return a.blanks > b.blanks
		default:
			return a.code > b.code
		}
	})
}

func dirLabel(rootAbs, displayRoot string, dirAbs string) string {
	if dirAbs == rootAbs {
		if displayRoot != "" {
			return displayRoot
		}
		return dirAbs
	}
	rel, err := filepath.Rel(rootAbs, dirAbs)
	if err != nil || rel == "." {
		return dirAbs
	}
	return rel
}

func writeDepthDirTable(opts *CmdOptions, linePrefix string, langs map[string]*langAgg) {
	if len(langs) == 0 {
		return
	}
	var rows []langRow
	for name, la := range langs {
		rows = append(rows, langRow{
			name: name, files: la.files, blanks: la.blanks, comments: la.comments, code: la.code,
		})
	}
	sortLangRows(rows, opts.SortTag)

	for _, r := range rows {
		fmt.Printf("%s%-27v %6v %14v %14v %14v\n",
			linePrefix, r.name, r.files, r.blanks, r.comments, r.code)
	}
}

func collectChildDirs(all map[string]map[string]*langAgg, parent string) []string {
	var out []string
	for k := range all {
		if k == parent {
			continue
		}
		if filepath.Dir(k) == parent {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func depthDirColorOn() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

func printDepthDirectoryLine(indent, label string) {
	if depthDirColorOn() {
		fmt.Printf("%s%s%s%s\n", indent, ansiBrightYellow, label, ansiReset)
		return
	}
	fmt.Printf("%s%s\n", indent, label)
}

func writeDepthTree(opts *CmdOptions, dirs map[string]map[string]*langAgg, rootAbs, displayRoot string, depth int) {
	var walk func(dirAbs string, level int)
	walk = func(dirAbs string, level int) {
		indent := strings.Repeat("    ", level)
		label := dirLabel(rootAbs, displayRoot, dirAbs)
		printDepthDirectoryLine(indent, label)
		linePrefix := indent + "    "
		writeDepthDirTable(opts, linePrefix, dirs[dirAbs])
		if level >= depth {
			return
		}
		for _, ch := range collectChildDirs(dirs, dirAbs) {
			walk(ch, level+1)
		}
	}
	walk(rootAbs, 0)
}

func writeDepthResult(opts *CmdOptions, result *gocloc.Result, paths []string, depth int) error {
	absRoots, err := absPathRoots(paths)
	if err != nil {
		return err
	}
	dirs := aggregateByDepth(result, absRoots, depth)
	if len(dirs) == 0 {
		return nil
	}
	for i, rootAbs := range absRoots {
		if i > 0 {
			fmt.Println()
		}
		display := paths[i]
		writeDepthTree(opts, dirs, rootAbs, display, depth)
	}
	return nil
}

func main() {
	var opts CmdOptions
	clocOpts := gocloc.NewClocOptions()
	// parse command line options
	parser := flags.NewParser(&opts, flags.Default)
	parser.Name = "gocloc"
	parser.Usage = "[OPTIONS] PATH[...]"

	paths, err := flags.Parse(&opts)
	if err != nil {
		return
	}

	// value for language result
	languages := gocloc.NewDefinedLanguages()

	if opts.ShowVersion {
		fmt.Printf("%s (%s)\n", Version, GitCommit)
		return
	}

	if opts.ShowLang {
		fmt.Println(languages.GetFormattedString())
		return
	}

	if len(paths) <= 0 {
		parser.WriteHelp(os.Stdout)
		return
	}

	// check sort tag option with other options
	if opts.ByFile && opts.SortTag == "files" {
		fmt.Println("`--sort files` option cannot be used in conjunction with the `--by-file` option")
		os.Exit(1)
	}

	if opts.Depth > 0 {
		if opts.ByFile {
			fmt.Println("`--depth` cannot be used with `--by-file`")
			os.Exit(1)
		}
		if opts.OutputType != OutputTypeDefault {
			fmt.Println("`--depth` only supports `--output-type default`")
			os.Exit(1)
		}
	}
	if opts.Depth < 0 {
		fmt.Println("`--depth` must be >= 0")
		os.Exit(1)
	}

	// setup option for exclude extensions
	for _, ext := range strings.Split(opts.ExcludeExt, ",") {
		e, ok := gocloc.Exts[ext]
		if ok {
			clocOpts.ExcludeExts[e] = struct{}{}
		} else {
			clocOpts.ExcludeExts[ext] = struct{}{}
		}
	}

	// directory and file matching options
	if opts.Match != "" {
		clocOpts.ReMatch = regexp.MustCompile(opts.Match)
	}
	if opts.NotMatch != "" {
		clocOpts.ReNotMatch = regexp.MustCompile(opts.NotMatch)
	}
	if opts.MatchDir != "" {
		clocOpts.ReMatchDir = regexp.MustCompile(opts.MatchDir)
	}
	if opts.NotMatchDir != "" {
		clocOpts.ReNotMatchDir = regexp.MustCompile(opts.NotMatchDir)
	}

	// setup option for include languages
	for _, lang := range strings.Split(opts.IncludeLang, ",") {
		if _, ok := languages.Langs[lang]; ok {
			clocOpts.IncludeLangs[lang] = struct{}{}
		}
	}

	clocOpts.Debug = opts.Debug
	clocOpts.SkipDuplicated = opts.SkipDuplicated
	clocOpts.Fullpath = opts.Fullpath

	processor := gocloc.NewProcessor(languages, clocOpts)
	result, err := processor.Analyze(paths)
	if err != nil {
		fmt.Printf("fail gocloc analyze. error: %v\n", err)
		return
	}

	if opts.Depth > 0 {
		if err := writeDepthResult(&opts, result, paths, opts.Depth); err != nil {
			fmt.Printf("depth output: %v\n", err)
			os.Exit(1)
		}
		return
	}

	builder := newOutputBuilder(result, &opts)
	builder.WriteResult()
}
