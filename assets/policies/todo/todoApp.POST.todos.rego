package todoApp.POST.todos

# Creating a todo is allowed when the directory holds the relation requested in input.resource for the user.

default allowed := false

allowed if {
	ds.check({
		"object_type": input.resource.object_type,
		"object_id": input.resource.object_id,
		"relation": input.resource.relation,
		"subject_type": "user",
		"subject_id": input.user.id,
	})
}
