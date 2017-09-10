# Turbo PR

Turbo PR is a tool for automatic Pull Request quality checks.

## Available checks

* Max amount of commits per PR
* Max/Min length of commit message subjects
* Max length of commit message body rows
* Regex matching commit subjects

## Configuration (add this as `turbo-pr.yaml` to your repo)

```yaml
pullRequest:
  maxAllowedCommits: 1

commit:
  maxSubjectLength: 72
  minSubjectLength: 10

  maxBodyRowLength: 50

  subjectMustMatchRegex:
  - "^(fea|fix|doc)\\([a-z0-9\\-]{2,30}\\)"
```
