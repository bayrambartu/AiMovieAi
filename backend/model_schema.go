package main

// Only API genres use a fixed vocabulary; user themes, moods and story descriptions remain free text.
func modelSchema(out any) map[string]any {
	text := func(description string) map[string]any {
		return map[string]any{"type": "string", "description": description, "maxLength": 240}
	}
	list := func(description string) map[string]any {
		return map[string]any{"type": "array", "items": map[string]any{"type": "string", "maxLength": 120}, "maxItems": 3, "description": description}
	}
	if _, ok := out.(*Intent); ok {
		return map[string]any{"type": "object", "properties": map[string]any{
			"story_arc":     text("Faithful concise English meaning of the movie request, preserving ALL specific subjects and constraints."),
			"keywords":      list("One to three English topic tags; preserve specific objects such as lighthouse rather than generic mood."),
			"search_titles": list("Up to two real original/English movie titles matching the specific request."),
		}, "required": []string{"story_arc", "keywords", "search_titles"}, "additionalProperties": false}
	}

	return map[string]any{"type": "object", "properties": map[string]any{
		"evaluations": map[string]any{"type": "array", "maxItems": 4, "items": map[string]any{"type": "object", "properties": map[string]any{
			"id":          map[string]any{"type": "integer"},
			"relevance":   map[string]any{"type": "number", "minimum": 0, "maximum": 1},
			"evidence_id": map[string]any{"type": "integer", "minimum": 0, "maximum": 15},
		}, "required": []string{"id", "relevance", "evidence_id"}, "additionalProperties": false}},
	}, "required": []string{"evaluations"}, "additionalProperties": false}
}

func modelContext(out any) int {
	if _, ok := out.(*Intent); ok {
		return 4096
	}
	return 4096
}
