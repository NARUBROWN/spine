<p align="center">
  <a href="README.md">한국어</a> | English
</p>

# 🦴 Spine
<p align="center">
  <img src="assets/spine-logo.png" alt="Spine" width="420" />
</p>

**Spine is the execution backbone of a backend system.**

Spine defines **how a request is resolved, executed, and completed** — explicitly.

**Spine is a backend framework that makes how requests are executed fully visible.**

---

# Learn Spine Easily
The official site is now live, where you can quickly understand Spine’s execution model and how to use it.<br>
[Spine Official Site](https://spine.na2ru2.me/en/)

## Spine's Concern

Most web frameworks hide the execution flow.

- Where a request starts
- Who creates the arguments
- When business code runs
- How return values become responses

All of this is buried under internal rules, conventions, and implicit behavior.

Spine does not hide this flow.  
It **fixes execution order and responsibilities in the code structure**.

---

## Example Project Built with Spine

👉 https://github.com/NARUBROWN/spine-user-demo

---

## What Spine Is Not

- ❌ Not an HTTP Engine
- ❌ Not a Router-centric framework
- ❌ Not an Annotation / Decorator framework
- ❌ Not Convention over Configuration
- ❌ Not delegating execution responsibility to Controllers

Spine is an **Execution Pipeline**.

---

## High-Level Architecture Overview

```
HTTP Transport (Echo)
        │
        ▼
ExecutionContext
        │
        ▼
Pipeline
  ├─ Router
  ├─ ParameterMeta Builder
  ├─ ArgumentResolver Chain
  ├─ Interceptor (preHandle)
  ├─ Invoker (Controller Method Call)
  ├─ ReturnValueHandler
  └─ Interceptor (postHandle)
        │
        ▼
ResponseWriter
```

This structure is not configuration — it is the **execution model itself**.

---

## Execution Pipeline

Every request **must** follow this order:

1. Enter Pipeline
2. Select HandlerMeta via Router
3. Build ParameterMeta
4. Run ArgumentResolver chain
5. Interceptor.preHandle
6. Call Controller Method (Invoker)
7. Run ReturnValueHandler
8. Interceptor.postHandle
9. Write response via ResponseWriter

This order is not hidden, and it does not change implicitly.

---

## Controller Design Philosophy

### Spine Principles

> **Controllers do not know the execution model.  
> But they must declare the source of inputs via types.**

Controllers **can depend on**:

- `path.*` : Path parameter semantic types
- `query.*` : Query parameter semantic types
- `httperr.*` : HTTP error semantic types

Controllers **do not depend on**:

- ExecutionContext
- Pipeline
- Router
- Resolver
- HTTP / Transport types

---

### Controller Example

```go
func (c *UserController) GetUser(userId path.Int) (User, error) {
    if userId.Value <= 0 {
        return User{}, httperr.BadRequest("Invalid user ID")
    }

    user, err := c.repo.FindByID(userId.Value)
    if err != nil {
        return User{}, httperr.NotFound("User not found")
    }

    return user, nil
}
```

Controllers:
- Do not know HTTP
- Do not know execution order
- Declare only the source of values

---

## Signature-as-Contract

In Spine, **method signatures are the API contract**.

- Input creation → `ArgumentResolver`
- Output handling → `ReturnValueHandler`

Changing the signature means changing the API.

Spine intentionally forbids:

- ❌ Annotation-based mapping
- ❌ Convention-based auto inference
- ❌ Implicit injection of primitive types

---

## Context Separation

Spine separates Context into two layers.

### ExecutionContext

- Controls execution flow
- Used only inside Router / Pipeline
- Not exposed to Controllers or Resolvers ❌

### RequestContext

- Parses inputs (Path / Query / Body)
- Used only in ArgumentResolvers
- Not exposed to Controllers ❌

---

## Path Parameter Binding Rule

Spine's path parameter binding is **order-based**.  
This is an **intentional and explicit contract**, considering Go's constraints.

### Rule

```
Route Path Key declaration order
=
Controller signature path.* parameter declaration order
```

### Example

```go
// Route
/users/:userId/posts/:postId

// Controller
func GetPost(userId path.Int, postId path.Int)
```

### Policy

- Name matching ❌
- Annotation ❌
- Primitive types ❌
- Fail fast on order mismatch

---

## Query Handling Principles

### Fixed-meaning Query

```go
func ListUsers(p query.Pagination)
```

Use the semantic types provided by Spine.

### Dynamic Query

```go
func SearchUsers(q query.Values)
```

- Provide raw query view
- User parses manually
- DTO auto-mapping ❌

---

## ReturnValueHandler & ResponseWriter

Controllers return values.

```go
return User{...}
```

Response generation is fully handled by `ReturnValueHandler`.  
Transport only implements `ResponseWriter`.

---

## License

MIT
