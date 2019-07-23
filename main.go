package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"gopkg.in/yaml.v2"
)

var (
	name     = flag.String("name", "", "app name")
	image    = flag.String("image", "", "docker image")
	env      = flag.String("env", "", "env file path")
	port     = flag.Int("port", 0, "port")
	domain   = flag.String("domain", "", "domain mapping")
	cert     = flag.Bool("cert", false, "request cert")
	certName = flag.String("cert-name", "", "cert secret name")
	hsts     = flag.String("hsts", "", "default, preload")
)

func main() {
	flag.Parse()

	if *name == "" {
		flag.Usage()
		fmt.Fprintln(flag.CommandLine.Output(), "")
		fmt.Fprintln(flag.CommandLine.Output(), "Examples:")
		fmt.Fprintln(flag.CommandLine.Output(), "$ kd -name app -image gcr.io/google-containers/echoserver:1.10 -port 8080 -domain echo.example.com -cert | kubectl replace --force -f -")
		return
	}

	var resources []map[string]interface{}

	if *image != "" {
		resources = append(resources, map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name": *name,
				"labels": map[string]string{
					"app": *name,
				},
			},
			"spec": map[string]interface{}{
				"replicas": 1,
				"selector": map[string]interface{}{
					"matchLabels": map[string]string{
						"app": *name,
					},
				},
				"strategy": map[string]interface{}{
					"type": "RollingUpdate",
					"rollingUpdate": map[string]interface{}{
						"maxSurge":       1,
						"maxUnavailable": 0,
					},
				},
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": *name,
						"labels": map[string]string{
							"app": *name,
						},
					},
					"spec": map[string]interface{}{
						// "affinity": map[string]interface{}{
						// 	"podAntiAffinity": map[string]interface{}{
						// 		"requiredDuringSchedulingIgnoreDuringExecution": map[string]interface{}{},
						// 	},
						// },
						"containers": []map[string]interface{}{
							{
								"name":  "app",
								"image": *image,
								// "imagePullPolicy": "Always",
								"env": loadEnv(*env),
							},
						},
					},
				},
			},
		})
	}

	if *port > 0 {
		resources = append(resources, map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name": *name,
				"labels": map[string]string{
					"app": *name,
				},
			},
			"spec": map[string]interface{}{
				"selector": map[string]string{
					"app": *name,
				},
				"ports": []map[string]interface{}{
					{
						"name":       "http",
						"port":       80,
						"targetPort": *port,
					},
				},
			},
		})
	}

	if *domain != "" {
		if *cert {
			if *certName == "" {
				*certName = domainToName(*domain)
			}

			resources = append(resources, map[string]interface{}{
				"apiVersion": "certmanager.k8s.io/v1alpha1",
				"kind":       "Certificate",
				"metadata": map[string]interface{}{
					"name": *name,
					"labels": map[string]string{
						"app": *name,
					},
				},
				"spec": map[string]interface{}{
					"acme": map[string]interface{}{
						"commonName": *domain,
						"dnsNames": []string{
							*domain,
						},
						"issuerRef": map[string]interface{}{
							"kind": "ClusterIssuer",
							"name": "letsencrypt",
						},
						"keyAlgorithm": "ecdsa",
						"keySize":      256,
						"secretName":   *certName,
						"config": []map[string]interface{}{
							{
								"domains": []string{
									*domain,
								},
								"http01": map[string]interface{}{
									"ingressClass": "parapet",
								},
							},
						},
					},
				},
			})
		}

		resources = append(resources, map[string]interface{}{
			"apiVersion": "extensions/v1beta1",
			"kind":       "Ingress",
			"metadata": map[string]interface{}{
				"name": *name,
				"labels": map[string]string{
					"app": *name,
				},
				"annotations": map[string]string{
					"kubernetes.io/ingress.class": "parapet",
					"parapet.moonrhythm.io/redirect-https": func() string {
						if *certName == "" {
							return "false"
						}
						return "true"
					}(),
					"parapet.moonrhythm.io/hsts": *hsts,
				},
			},
			"spec": map[string]interface{}{
				"rules": []map[string]interface{}{
					{
						"host": *domain,
						"http": map[string]interface{}{
							"paths": []map[string]interface{}{
								{
									"backend": map[string]interface{}{
										"serviceName": *name,
										"servicePort": "http",
									},
								},
							},
						},
					},
				},
				"tls": func() []map[string]interface{} {
					if *certName == "" {
						return []map[string]interface{}{}
					}
					return []map[string]interface{}{
						{
							"secretName": *certName,
						},
					}
				}(),
			},
		})
	}

	yaml.NewEncoder(os.Stdout).Encode(map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "List",
		"items":      resources,
	})
}

func loadEnv(filename string) interface{} {
	xs := make([]map[string]string, 0)

	if filename == "" {
		return xs
	}

	b, err := ioutil.ReadFile(filename)
	if err != nil {
		panic(err)
	}
	if len(b) == 0 {
		return xs
	}

	ps := strings.Split(string(b), "\n")

	for _, p := range ps {
		l := strings.SplitN(p, "=", 2)
		if len(l) != 2 {
			continue
		}
		n := strings.TrimSpace(l[0])
		v := strings.TrimSpace(l[1])
		if n == "" {
			continue
		}
		xs = append(xs, map[string]string{
			"name":  n,
			"value": v,
		})
	}
	return xs
}

func domainToName(domain string) string {
	domain = strings.ReplaceAll(domain, ".", "-")
	return domain
}
