package authz

default allow := false

allow if input.user.id == "U0ALLOWED"
