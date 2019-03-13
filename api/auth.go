package api

import (
	"regexp"
)

// whether an api endpoint with the given url pattern is allowed or not
type permissionPolicy map[string]bool

// whether the user is trusted or not
type trustMap map[string]bool

func getPermissionPolicy(trusted bool) permissionPolicy {
	if trusted {
		return permissionPolicy{
			"id": true,
		}
	}

	return permissionPolicy{}
}

// func authMiddlewareFunc(policy permissionPolicy) func(next http.Handler) http.Handler {
// 	return func(next http.Handler) http.Handler {
// 		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 			match, _ := regexp.MatchString("/users.*", r.URL.Path)
// 			next.ServeHTTP(w, r)
// 		})
// 	}
// }

// WithAuthorization wraps the handler with authorization check
// func WithAuthorization(policy permissionPolicy, h http.Handler) http.Handler {
// 	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		var match bool
// 		for k := range policy {
// 			match, _ = regexp.MatchString(k, r.URL.Path)
// 			if !match {
// 				continue
// 			}
// 			break
// 		}

// 		if !match {
// 			resp, err := unauthorizedEndpointResp(r.URL.Path)
// 			if err != nil {
// 				logger.Error(err)
// 				return
// 			}
// 			http.Error(w, resp, 401)
// 			return
// 		}
// 		h.ServeHTTP(w, r)
// 	})
// }

// func unauthorizedEndpointResp(path string) (string, error) {
// 	apiError := &Error{
// 		Code:    401,
// 		Message: "Unauthorized to access path " + path,
// 	}
// 	resp, err := json.Marshal(apiError)
// 	return string(resp), err
// }

// IsAuthorized checks if a path is allowed or not given whether the
// peer is trusted or untrusted
func IsAuthorized(trusted bool, path string) bool {
	policy := getPermissionPolicy(trusted)

	var match bool
	for k := range policy {
		match, _ = regexp.MatchString(k, path)
		if !match {
			continue
		}
		break
	}

	return match
}
