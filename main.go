package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/container/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

var (
	project     = flag.String("project", "", "Name of the project (empty means look it up)")
	location    = flag.String("location", "", "Location of the cluster (empty means look it up)")
	clusterName = flag.String("cluster", "", "Name of the cluster")

	get = flag.Bool("get", false, "If true, print auth information")
)

func main() {
	flag.Parse()
	ctx := context.Background()

	// get a token
	ts, err := google.DefaultTokenSource(ctx)
	if err != nil {
		log.Fatal(err)
	}
	tok, err := ts.Token()
	if err != nil {
		log.Fatal(err)
	}

	if *get {
		// just print the token
		if err := json.NewEncoder(os.Stdout).Encode(map[string]string{
			"access_token": tok.AccessToken,
			"token_expiry": tok.Expiry.Format(time.RFC3339),
		}); err != nil {
			log.Fatal(err)
		}
		return
	}

	// get the GKE cluster
	gke, err := container.NewService(ctx)
	if err != nil {
		log.Fatal(err)
	}
	cluster, err := gke.Projects.Locations.Clusters.Get(fmt.Sprintf("projects/%s/locations/%s/clusters/%s", *project, *location, *clusterName)).Do()
	if err != nil {
		log.Fatal(err)
	}

	// load the current kubeconfig
	fn := os.Getenv("KUBECONFIG")
	if fn == "" {
		fn = filepath.Join(os.Getenv("HOME"), ".kube", "config")
	}
	cfg, err := clientcmd.LoadFromFile(fn)
	if err != nil {
		log.Fatal(err)
	}

	// add user
	key := fmt.Sprintf("gke_%s_%s_%s", *project, *location, *clusterName)
	cfg.AuthInfos[key] = &api.AuthInfo{
		AuthProvider: &api.AuthProviderConfig{
			Config: map[string]string{
				"access-token": tok.AccessToken,
				"cmd-args":     "--get",
				"cmd-path":     os.Args[0],
				"expiry":       tok.Expiry.Format(time.RFC3339),
				"expiry-key":   "{.token_expiry}",
				"token-key":    "{.access_token}",
			},
			Name: "gcp",
		},
	}

	// add cluster
	dec, err := base64.StdEncoding.DecodeString(cluster.MasterAuth.ClusterCaCertificate)
	if err != nil {
		log.Fatal(err)
	}
	cfg.Clusters[key] = &api.Cluster{
		Server:                   "https://" + cluster.Endpoint,
		CertificateAuthorityData: dec,
	}

	// update current context
	cfg.Contexts[key] = &api.Context{
		AuthInfo: key,
		Cluster:  key,
	}
	cfg.CurrentContext = key

	// write the file back
	if err := clientcmd.WriteToFile(*cfg, fn); err != nil {
		log.Fatal(err)
	}
}