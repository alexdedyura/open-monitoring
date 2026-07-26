//go:build windows

package app

import toast "git.sr.ht/~jackmordaunt/go-toast/v2"

// pushToast shows a native Windows notification. Failures are ignored: the
// in-app toast has already been emitted, and there is nobody to report to.
func pushToast(title, body string) {
	_ = (&toast.Notification{
		AppID: "Open Monitoring",
		Title: title,
		Body:  body,
	}).Push()
}
