# Use MuAPI for image generation

Denova's custom image endpoint can connect to services that implement the OpenAI Images API. MuAPI exposes `POST /v1/images/generations` in that format, so you can add it as a custom image connection without a provider-specific plugin.

## Configure the connection

1. Open **Settings** and add an image endpoint.
2. Choose **Custom** as the provider and **OpenAI Images API** as the protocol.
3. Set the base URL to `https://api.muapi.ai/v1` and enter your MuAPI API key.
4. Add a model profile with a MuAPI image model ID, such as `flux-schnell`, and choose it as the default image model.
5. Use the square `1024x1024` size for the first generation. MuAPI's OpenAI-compatible guide documents this size and returns generated images as URLs.

MuAPI's model catalogue and current model IDs are listed in its [OpenAI-compatible API guide](https://muapi.ai/docs/openai-compatible). Keep the key in Denova's endpoint settings; do not put it in a prompt or model name.

## Scope and connection checks

This setup covers text-to-image generation through MuAPI's OpenAI-compatible `images/generations` endpoint. MuAPI's guide does not document the matching `images/edits` route, so do not assume reference-image editing will work through this connection. MuAPI's provider-specific image-editing endpoints use their own request format.

The **Test connection** action sends a real, minimal image-generation request. It does not save the returned image, but the request may consume MuAPI credits. Use it only when you are ready to submit a generation.
