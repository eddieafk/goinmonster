![goinmonster](https://raw.githubusercontent.com/eddieafk/goinmonster/main/docs/img/join_monster.png)

**goinmonster** is a GraphQL to SQL code generator and query execution toolkit for Go. It allows you to map GraphQL schemas to SQL databases and generate efficient Go code for query resolution. It's a [join-monster]() derivative project.

## Roadmap 

- [X] PostgreSQL dialect.
- [ ] Other dialects; mysql/oracle.
- [ ] Enhancements on benchmarks.


## Getting Started

### 1. Initialize a New Project
```
goinmonster init
```
This creates a `goinmonster.yaml` config file and an example `schema.graphqls` in your project directory.

### 2. Define Your GraphQL Schema
Place your schema in a file (e.g., `schema.graphqls`).

### 3. Generate Go Code
```
goinmonster generate
```
This generates Go code for your GraphQL types, resolvers, and SQL mapping.

### 4. Build and Run
You should define your own database as it's shown in [test/](test/) package.

### CLI Examples
```
goinmonster init
goinmonster generate
goinmonster gen --config goinmonster.yaml
```

## Configuration
By default, goinmonster looks for `goinmonster.yaml` in the current directory. Use `--config` to specify a different configuration file.


## License
Apache 2.0
