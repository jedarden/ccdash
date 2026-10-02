# ccdash documentation

The repository keeps user-facing documentation at the root and supporting
material here. Start with the page that matches the question:

| Need | Read |
| --- | --- |
| Product overview, installation, flags, configuration, and notifications | [`../README.md`](../README.md) |
| Runtime components and data flow | [`architecture.md`](architecture.md) |
| Accepted or proposed design decisions | [`adrs/README.md`](adrs/README.md) |
| Historical research for the shipped worker/session visualization | [`research/README.md`](research/README.md) |
| Small operational or implementation notes | [`notes/README.md`](notes/README.md) |
| Current implementation plan and review backlog | [`plan/plan.md`](plan/plan.md) |

## Layout

```text
README.md                 Product documentation and usage reference
docs/
├── README.md             This index
├── architecture.md       Current collectors, cache, and render loop
├── adrs/                 Architecture decision records
├── images/               Documentation assets
├── k8s/                  CI workflow reference material
├── notes/                Short operational and implementation notes
├── plan/plan.md          Living implementation plan
└── research/             Historical research and shipped-feature rationale
```

## Contributing

- Keep durable architecture and behavior descriptions in `architecture.md`.
- Record a significant trade-off in `adrs/` and add it to that directory's
  index.
- Put exploratory or historical material in `research/`; label it as such
  when the implementation has shipped.
- Use `notes/` for concise, maintainable notes rather than bead scratch
  records or duplicate changelogs.
- Update this index when adding a documentation area or a durable entry point.
