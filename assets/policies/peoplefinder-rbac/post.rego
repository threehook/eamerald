package peoplefinder.POST.api.users

# Creating an employee is allowed for admins. It is visible and enabled when allowed.

import input.user.properties.roles as user_roles

default allowed := false

default visible := false

default enabled := false

allowed if "admin" in user_roles

visible if allowed

enabled if allowed
