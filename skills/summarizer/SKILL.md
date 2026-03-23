---
name: summarizer
description: Summarizes the content of a text file.
auto_trigger: false
---

# Summarizer

Reads a text file and generates a concise summary of its content using the
configured LLM provider.

## Steps

1. Read the target file
2. Send content to the LLM with a summarization prompt
3. Return the summary to the user
