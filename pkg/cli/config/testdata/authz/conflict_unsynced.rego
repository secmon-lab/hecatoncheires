# Both rules hold only when the user has no synced record (email is ""),
# so only the unsynced trial input produces conflicting values.
package authz

allow := true if input.user.id != ""

allow := false if input.user.email == ""
