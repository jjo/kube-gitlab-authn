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
func get_groups(client *gitlab.Client, user *gitlab.User, root_group string) ([]string, error) {
	// Get user's group
	opt := &gitlab.ListGroupsOptions{
		Search: &root_group,
		// MinAccessLevel: gitlab.AccessLevel(gitlab.AccessLevelValue(30)), // Developer
	}
	groups, _, err := client.Groups.ListGroups(opt)
	if err != nil {
		return nil, fmt.Errorf("[Error] ListGroups: %s", err.Error())
	}

	var groups_paths []string
	// If root_group is not empty
	// - User must below to root_group
	// - groups_paths will also include all user subgroups
	if root_group != "" {
		root_group_id := -1
		for _, g := range groups {
			if root_group == g.Path {
				root_group_id = g.ID
			}
		}
		if root_group_id == -1 {
			return nil, fmt.Errorf("[Error] user='%s' is not a member of root_group='%s'", user.Username, root_group)
		}
		opt := &gitlab.ListSubgroupsOptions{
			// MinAccessLevel: gitlab.AccessLevel(gitlab.AccessLevelValue(30)), // Developer
		}
		subgroups, _, err := client.Groups.ListSubgroups(root_group_id, opt)
		if err != nil {
			return nil, fmt.Errorf("[Error] ListSubgroups('%s'): %s", root_group, err.Error())
		}
		// Return groups_paths = [root_group] + subgroups
		groups_paths = append(groups_paths, root_group)
		for _, g := range subgroups {
			groups_paths = append(groups_paths, g.FullPath)
		}
	} else {
		for _, g := range groups {
			groups_paths = append(groups_paths, g.FullPath)
		}
	}

	return groups_paths, nil
}

func main() {
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

		all_group_path, err := get_groups(client, user, os.Getenv("GITLAB_ROOT_GROUP"))
		if err != nil {
			unauthorized(w, err.Error())
			return
		}
		// Set the TokenReviewStatus
		log.Printf("[Success] login as %s, groups: %v", user.Username, all_group_path)
		w.WriteHeader(http.StatusOK)
		trs := authentication.TokenReviewStatus{
			Authenticated: true,
			User: authentication.UserInfo{
				Username: user.Username,
				UID:      user.Username,
				Groups:   all_group_path,
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
