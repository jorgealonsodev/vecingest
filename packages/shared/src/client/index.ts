// GENERATED FILE — DO NOT EDIT.
// Source: api/openapi/openapi.yaml (via openapi-typescript)
// Regenerate with `make gen` (api-contract-generation: Generation Chain).

import createClient from "openapi-fetch";
import type { Client, ClientOptions } from "openapi-fetch";
import type { paths } from "./openapi-types.js";

export type { paths } from "./openapi-types.js";
export type { ClientOptions } from "openapi-fetch";

/** Typed fetch client for the Vecingest API, generated from openapi.yaml. */
export function createApiClient(options?: ClientOptions): Client<paths> {
  return createClient<paths>(options);
}

export type ApiClient = Client<paths>;
