package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"

	"github.com/xanzy/go-gitlab"

	authentication "k8s.io/api/authentication/v1beta1"
)

type byLen []string

func (a byLen) Len() int           { return len(a) }
func (a byLen) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byLen) Less(i, j int) bool { return len(a[i]) < len(a[j]) }

func unauthorized(w http.ResponseWriter, format string, args ...interface{}) {
	log.Printf(format, args...)
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"apiVersion": "authentication.k8s.io/v1beta1",
		"kind":       "TokenReview",
		"status": authentication.TokenReviewStatus{
			Authenticated: false,
		},
	})
}
func getGroups(client *gitlab.Client, user *gitlab.User, groupRe *regexp.Regexp, projectRe *regexp.Regexp) ([]string, error) {
	// Get user's group
	opt := &gitlab.ListGroupsOptions{
		ListOptions: gitlab.ListOptions{
			PerPage: 10000,
		},
		// MinAccessLevel: gitlab.AccessLevel(gitlab.AccessLevelValue(30)), // Developer
	}
	groups, _, err := client.Groups.ListGroups(opt)
	if err != nil {
		return nil, fmt.Errorf("[Error] ListGroups: %s", err.Error())
	}

	var groupsPaths []string
	for _, g := range groups {
		if groupRe == nil || groupRe.Find([]byte(g.FullPath)) != nil {
			groupsPaths = append(groupsPaths, g.FullPath)
			if projectRe != nil {
				projects, _, err := client.Groups.ListGroupProjects(g.ID, nil)
				if err != nil {
					return nil, fmt.Errorf("[Error] ListGroupProjects: %s", err.Error())
				}
				for _, p := range projects {
					if projectRe.Find([]byte(p.PathWithNamespace)) != nil {
						groupsPaths = append(groupsPaths, p.PathWithNamespace)
					}
				}
			}
		}
	}

	// If groupRe is set, then user MUST belong to at least one matching group
	if groupRe != nil {
		if len(groupsPaths) == 0 {
			return nil, fmt.Errorf("[Error] User '%s' doesn't below to any matching group", user.Username)
		}
		sort.Sort(byLen(groupsPaths))
	}

	return groupsPaths, nil
}

func main() {
	apiEp := os.Getenv("GITLAB_API_ENDPOINT")
	if apiEp == "" {
		log.Fatalf("GITLAB_API_ENDPOINT env var empty")
	}
	gitlabGroupRe := os.Getenv("GITLAB_GROUP_RE")
	gitlabProjectRe := os.Getenv("GITLAB_PROJECT_RE")
	gitlabRootGroup := os.Getenv("GITLAB_ROOT_GROUP")
	// GITLAB_GROUP_RE takes precedence
	// else if GITLAB_ROOT_GROUP is set,
	//   then build equivalent regexp from it
	// If none is set, then not group matching enforcement will be done
	// If (and only if) GITLAB_PROJECT_RE is set, then *also*
	// add projects matching this regex for their full path (ie GROUP/PROJECT)
	var groupRe *regexp.Regexp
	var projectRe *regexp.Regexp
	if gitlabGroupRe == "" {
		if gitlabRootGroup != "" {
			gitlabGroupRe = fmt.Sprintf("^(%s)(/.+)?$", gitlabRootGroup)
		}
	}
	if gitlabGroupRe != "" {
		groupRe = regexp.MustCompile(gitlabGroupRe)
	}
	if gitlabProjectRe != "" {
		projectRe = regexp.MustCompile(gitlabProjectRe)
	}
	log.Println("Gitlab Authn Webhook:", apiEp)
	log.Printf("Using gitlabRootGroup: '%s'.", gitlabRootGroup)
	log.Printf("Using gitlabGroupRe: '%s'.", gitlabGroupRe)
	log.Printf("Using gitlabProjectRe: '%s'.", gitlabProjectRe)

	http.HandleFunc("/authenticate", func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		var tr authentication.TokenReview
		err := decoder.Decode(&tr)
		if err != nil {
			unauthorized(w, "[Error] decoding request: %s", err.Error())
			return
		}

		client := gitlab.NewClient(nil, tr.Spec.Token)
		client.SetBaseURL(apiEp)

		// Get user
		user, _, err := client.Users.CurrentUser()
		if err != nil {
			unauthorized(w, "[Error] invalid token: %s", err.Error())
			return
		}

		groups, err := getGroups(client, user, groupRe, projectRe)
		if err != nil {
			unauthorized(w, err.Error())
			return
		}
		// Set the TokenReviewStatus
		log.Printf("[Success] login as %s, groups: %v", user.Username, groups)
		w.WriteHeader(http.StatusOK)
		trs := authentication.TokenReviewStatus{
			Authenticated: true,
			User: authentication.UserInfo{
				Username: user.Username,
				UID:      user.Username,
				Groups:   groups,
			},
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"apiVersion": "authentication.k8s.io/v1beta1",
			"kind":       "TokenReview",
			"status":     trs,
		})
	})
	log.Fatal(http.ListenAndServe(":3000", nil))
}
