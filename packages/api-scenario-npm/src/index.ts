// data/openapi.yaml から openapi-typescript で生成された components / paths の re-export。
// 利用者は本ファイルが re-export する schema 型を import すること。

import type { components, paths } from "./openapi.gen";

export type { components, paths };

type Schemas = components["schemas"];

export type HealthResponse = Schemas["HealthResponse"];
export type EpisodeWithStatus = Schemas["EpisodeWithStatus"];
export type EpisodesListResponse = Schemas["EpisodesListResponse"];
export type LockReason = Schemas["LockReason"];
export type LockReasonType = Schemas["LockReasonType"];
export type ScenarioScriptResponse = Schemas["ScenarioScriptResponse"];
export type ScenarioCompleteResponse = Schemas["ScenarioCompleteResponse"];
export type OnboardingScriptResponse = Schemas["OnboardingScriptResponse"];
export type OnboardingCompleteResponse = Schemas["OnboardingCompleteResponse"];
export type OnboardingNameRequest = Schemas["OnboardingNameRequest"];
export type OnboardingFactionRequest = Schemas["OnboardingFactionRequest"];
