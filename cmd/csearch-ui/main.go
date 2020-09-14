package main

import (
	"context"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/google/codesearch/cmd/cindex-serve/service"
	"github.com/google/codesearch/expr"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

var (
	indexMetadataService = flag.String("index_metadata_service", "localhost:8801", "Location to find the IndexMetadataService.")
	searchShardService   = flag.String("search_shard_service", "localhost:8801", "Location to find the SearchShardService.")

	port = flag.Int("port", 0, "Port to serve on")
)

func autoErr(f func(http.ResponseWriter, *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if err := f(w, req); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
		}
	}
}

type server struct {
	meta   service.IndexMetadataServiceClient
	search service.SearchShardServiceClient
}

func (s *server) err(w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte(err.Error()))
}

var searchPage = template.Must(template.New("search").Parse(`
<head>
	<style>
		a {
			color: inherit;
			text-decoration: none;
		}
		body {
			margin: 0;
		}
		.search-header {
			padding: 20px;
			text-align: center;
		}
		.search-box {
			width: 75%;
		}
		.stats {
			text-align: right;
		}
		.file {
			margin: 15px 0px;
		}
		.file-header {
			background-color: #EEEEFF;
			padding: 3px 15px;
		}
		.snippet {
		  font-family: monospace;
			padding: 2px 15px;
		}
		.lineno {
			display: inline-block;
			text-align: right;
			width: 3em;
		}
	</style>
</head>
<body>
	<div class="search-header">
		<form action="/search" method="get">
			<input class="search-box" type="text" name="q" value="{{.Query}}">
			<input type="submit" value="Search">
		</form>
	</div>
	<div class="stats">Showing {{.ResultCount}} results in {{.Delay}}</div>
	{{range .Results}}{{range .}}
		{{$basePath := printf "https://%s/blob/%s/%s" .Repo .Commit .Doc.Path}}
		<div class="file">
			<a href="{{$basePath}}"><div class="file-header">{{.Repo}}/{{.Doc.Path}}</div></a>
			{{range .Doc.Snippets}}
				<a href="{{$basePath}}#L{{.FirstLine}}">
					<div class="snippet">
						<span class="lineno">{{.FirstLine}}</span>:<span class="code">{{.Lines}}</span>
					</div>
				</a>
			{{end}}
		</div>
	{{end}}{{end}}
</body>
`))

func (s *server) Search(w http.ResponseWriter, req *http.Request) error {
	start := time.Now()
	ctx := req.Context()

	query := req.FormValue("q")

	type result struct {
		Repo   string
		Commit string
		Doc    *service.Doc
	}

	data := struct {
		Query       string
		Results     [][]result
		ResultCount int
		Delay       time.Duration
	}{
		Query: query,
	}

	if query != "" {
		expr, err := expr.Parse(query)
		if err != nil {
			return err
		}

		resp, err := s.meta.ListRepoRefs(ctx, &service.ListRepoRefsRequest{})
		if err != nil {
			return err
		}

		data.Results = make([][]result, len(resp.GetRepoRefs()))
		if err := func() error {
			g, ctx := errgroup.WithContext(ctx)
			for i, repoRef := range resp.GetRepoRefs() {
				i, repoRef := i, repoRef
				g.Go(func() error {
					resp, err := s.search.SearchShard(ctx, &service.SearchShardRequest{
						ShardId:     repoRef.GetShard().GetId(),
						ShardSha256: repoRef.GetShard().GetSha256(),
						Expression:  expr,
						PathPrefix:  repoRef.GetRepoName() + "/",
					})
					if err != nil {
						return err
					}
					for _, doc := range resp.GetDocs() {
						data.Results[i] = append(data.Results[i], result{
							Repo:   repoRef.GetRepoName(),
							Commit: repoRef.GetCommitHash(),
							Doc:    doc,
						})
						data.ResultCount += len(doc.GetSnippets())
						// urlBase := fmt.Sprintf("%v/%v", urlBase, doc.GetPath())
						// for _, snip := range doc.GetSnippets() {
						// 	fmt.Printf("%s#L%d\n\n%s\n", urlBase, snip.GetFirstLine(), snip.GetLines())
						// }
					}
					return nil
				})
			}
			return g.Wait()
		}(); err != nil {
			return err
		}
	}

	data.Delay = time.Since(start)

	return searchPage.Execute(w, data)
}

func main() {
	flag.Parse()

	ctx := context.Background()

	conn, err := grpc.DialContext(ctx, *indexMetadataService, grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	indexMetadata := service.NewIndexMetadataServiceClient(conn)

	if *indexMetadataService != *searchShardService {
		conn, err = grpc.DialContext(ctx, *searchShardService, grpc.WithInsecure())
		if err != nil {
			log.Fatal(err)
		}
		defer conn.Close()
	}
	searchShard := service.NewSearchShardServiceClient(conn)

	s := server{
		meta:   indexMetadata,
		search: searchShard,
	}

	mux := http.NewServeMux()
	mux.Handle("/search", autoErr(s.Search))

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	log.Printf("Serving on %v", lis.Addr())
	if err := http.Serve(lis, mux); err != nil {
		log.Fatal(err)
	}
}
