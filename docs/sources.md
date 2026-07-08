# Sources and rationale

This template is based on the following public guidance:

- Codex subagents: https://developers.openai.com/codex/subagents
- Codex configuration reference: https://developers.openai.com/codex/config-reference
- Codex model recommendations: https://developers.openai.com/codex/models
- Codex CLI review command: https://developers.openai.com/codex/cli/features
- Codex GitHub code review: https://developers.openai.com/codex/integrations/github
- Google Engineering Practices code review guidance: https://google.github.io/eng-practices/review/reviewer/looking-for.html

Design choices:

- The custom reviewer uses `sandbox_mode = "read-only"` because a reviewer should not silently edit the code under review.
- The reviewer uses `model_reasoning_effort = "high"` because review requires tracing contracts, callers, edge cases, and tests.
- The default model is `gpt-5.5`, which OpenAI recommends for most Codex tasks.
- Review output is severity-ranked to keep style-only feedback from burying real risks.
