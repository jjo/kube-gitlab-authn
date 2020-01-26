package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/xanzy/go-gitlab"

	authentication "k8s.io/api/authentication/v1beta1"
)

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
func getGroups(client *gitlab.Client, user *gitlab.User, rootGroup string) ([]string, error) {
	// Get user's group
	opt := &gitlab.ListGroupsOptions{
		Search: &rootGroup,
		// MinAccessLevel: gitlab.AccessLevel(gitlab.AccessLevelValue(30)), // Developer
	}
	groups, _, err := client.Groups.ListGroups(opt)
	if err != nil {
		return nil, fmt.Errorf("[Error] ListGroups: %s", err.Error())
	}

	var groupsPaths []string
	// If rootGroup is not empty
	// - User must below to rootGroup
	// - groupsPaths will also include all user subgroups
	if rootGroup != "" {
		rootGroupID := -1
		for _, g := range groups {
			if rootGroup == g.Path {
				rootGroupID = g.ID
			}
		}
		if rootGroupID == -1 {
			return nil, fmt.Errorf("[Error] user='%s' is not a member of rootGroup='%s'", user.Username, rootGroup)
		}
		opt := &gitlab.ListSubgroupsOptions{
			// MinAccessLevel: gitlab.AccessLevel(gitlab.AccessLevelValue(30)), // Developer
		}
		subgroups, _, err := client.Groups.ListSubgroups(rootGroupID, opt)
		if err != nil {
			return nil, fmt.Errorf("[Error] ListSubgroups('%s'): %s", rootGroup, err.Error())
		}
		// Return groupsPaths = [rootGroup] + subgroups
		groupsPaths = append(groupsPaths, rootGroup)
		for _, g := range subgroups {
			groupsPaths = append(groupsPaths, g.FullPath)
		}
	} else {
		for _, g := range groups {
			groupsPaths = append(groupsPaths, g.FullPath)
		}
	}

	return groupsPaths, nil
}

func main() {
	apiEp := os.Getenv("GITLAB_API_ENDPOINT")
	if apiEp == "" {
		log.Fatalf("GITLAB_API_ENDPOINT env var empty")
	}
	log.Println("Gitlab Authn Webhook:", os.Getenv("GITLAB_API_ENDPOINT"))
	http.HandleFunc("/authenticate", func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		var tr authentication.TokenReview
		err := decoder.Decode(&tr)
		if err != nil {
			unauthorized(w, "[Error] decoding request: %s", err.Error())
			return
		}

		client := gitlab.NewClient(nil, tr.Spec.Token)
		client.SetBaseURL(os.Getenv("GITLAB_API_ENDPOINT"))

		// Get user
		user, _, err := client.Users.CurrentUser()
		if err != nil {
			unauthorized(w, "[Error]: %s", err.Error())
			return
		}

		allGroupsPaths, err := getGroups(client, user, os.Getenv("GITLAB_ROOT_GROUP"))
		if err != nil {
			unauthorized(w, err.Error())
			return
		}
		// Set the TokenReviewStatus
		log.Printf("[Success] login as %s, groups: %v", user.Username, allGroupsPaths)
		w.WriteHeader(http.StatusOK)
		trs := authentication.TokenReviewStatus{
			Authenticated: true,
			User: authentication.UserInfo{
				Username: user.Username,
				UID:      user.Username,
				Groups:   allGroupsPaths,
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
