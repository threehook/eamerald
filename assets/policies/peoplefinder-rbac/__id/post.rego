package peoplefinder.POST.api.users.__id

# Updating an employee through POST is allowed for admins. It is always visible, and enabled when allowed.

import input.user.properties.roles as user_roles

default allowed := false

default visible := true

default enabled := false

allowed if "admin" in user_roles

enabled if allowed
