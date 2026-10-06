package peoplefinder.DELETE.api.users.__id

# Deleting an employee is allowed for admins. It is visible to editors and admins, and enabled when allowed.

import input.user.properties.roles as user_roles

default allowed := false

default visible := false

default enabled := false

allowed if "admin" in user_roles

visible if {
	some role in {"editor", "admin"}
	role in user_roles
}

enabled if allowed
