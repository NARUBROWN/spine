# Spine documentation

English is the default documentation language. [Korean originals](ko/README.md) are retained under `docs/ko/`; historical source text is marked as such. Other README translations are available in [Japanese](ja/README.md), [Simplified Chinese](zh-Hans/README.md) and [Traditional Chinese](zh-Hant/README.md).

This branch targets `v0.7.6` and contains source APIs newer than the previously published `v0.7.6-preview.1` tag. Select matching core, platform and CLI contracts before running an application.

## Start with an example

- [Minimal HTTP and WebSocket demo](examples/demo.md)
- [Platform reference application](examples/reference.md)
- [WebSocket echo and HTTP push](examples/websocket.md)
- [Publish-only relay](examples/publish-only.md)
- [Kafka publisher](examples/kafka-publish.md)
- [Security configuration](security-configuration.md)

## Understand execution

- [Context objects, public views and lifetimes](guides/contexts.md)
- [HTTP context ownership](migration/http-context.md)
- [Controller callbacks](migration/controller-routes.md)
- [Constructor roles](migration/component-contracts.md)
- [Explicit HTTP inputs](migration/structured-inputs.md)
- [Application DI Publisher](migration/app-publisher.md)
- [WebSocket callbacks, Sender and hooks](migration/websocket-dx.md)

## Platform reference

- [Contract overview](reference/contracts.md)
- [HTTP](reference/http-platform.md) and [input parsing](reference/http-inputs.md)
- [WebSocket](reference/websocket-platform.md)
- [Events](reference/event-platform.md)
- [Validation](reference/validation.md)
- [Runtime OpenAPI](reference/openapi.md) and [embedded UIs](reference/openapi-ui.md)
- [Compatibility imports](reference/adapter-compatibility.md)

## Verify and maintain

- [Test commands and module boundaries](testing/README.md)
- [HTTP platform comparison](testing/http-platforms.md)
- [Raw HTTP contract probes](testing/http-contract.md)
- [OpenAPI verification](testing/openapi.md)
- [Broker combinations](testing/platforms.md)
- [Release operations](maintenance/releasing.md)
- [Discord tag notifications](maintenance/discord-tag.md)
- [Asset attribution](attribution.md)

## Migration history

Use [v0.7.6 source migration](migration/v0.7.6.md) for current development. Historical guides remain for [v0.7.5](migration/v0.7.5.md), [v0.7.2](migration/v0.7.2.md), [v0.7.1](migration/v0.7.1.md), [v0.7.0](migration/v0.7.0.md), [v0.6.6](migration/v0.6.6.md), [v0.6.5](migration/v0.6.5.md), [v0.5.2](migration/v0.5.2.md), [v0.5.1](migration/v0.5.1.md) and [v0.5](migration/v0.5.md).

All Markdown documentation belongs under `docs/`. The only root exceptions are the GitHub `README.md` and the `AGENTS.md`/`.AGENT.md` instruction files. Package directories use Go source comments instead of duplicate README files.
