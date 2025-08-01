// Prometheus API DTOs, possibly to be autogenerated from openapi spec in the near future

import { DataQuery, RelativeTimeRange } from '@grafana/data';
import { ExpressionQuery } from 'app/features/expressions/types';

import { AlertGroupTotals, AlertInstanceTotals } from './unified-alerting';

export type Labels = Record<string, string>;
export type Annotations = Record<string, string>;

export enum PromAlertingRuleState {
  Firing = 'firing',
  Inactive = 'inactive',
  Pending = 'pending',
  Recovering = 'recovering',
  Unknown = 'unknown',
}

export enum GrafanaAlertState {
  Normal = 'Normal',
  Alerting = 'Alerting',
  Pending = 'Pending',
  Recovering = 'Recovering',
  NoData = 'NoData',
  Error = 'Error',
}

type GrafanaAlertStateReason = ` (${string})` | '';

export type GrafanaAlertStateWithReason = `${GrafanaAlertState}${GrafanaAlertStateReason}`;

export function isPromAlertingRuleState(state: string): state is PromAlertingRuleState {
  return Object.values<string>(PromAlertingRuleState).includes(state);
}

export function isGrafanaAlertState(state: string): state is GrafanaAlertState {
  return Object.values(GrafanaAlertState).some((promState) => promState === state);
}

/** We need this to disambiguate the union PromAlertingRuleState | GrafanaAlertStateWithReason
 */
export function isAlertStateWithReason(
  state: PromAlertingRuleState | GrafanaAlertStateWithReason
): state is GrafanaAlertStateWithReason {
  const propAlertingRuleStateValues: string[] = Object.values(PromAlertingRuleState);
  return state !== null && state !== undefined && !propAlertingRuleStateValues.includes(state);
}

export function mapStateWithReasonToReason(state: GrafanaAlertStateWithReason): string {
  const match = state.match(/\((.*?)\)/);
  return match ? match[1] : '';
}

type StateWithReasonToBaseStateReturnType<T> = T extends GrafanaAlertStateWithReason
  ? GrafanaAlertState
  : T extends PromAlertingRuleState
    ? PromAlertingRuleState
    : never;

export function mapStateWithReasonToBaseState(
  state: GrafanaAlertStateWithReason | PromAlertingRuleState
): StateWithReasonToBaseStateReturnType<GrafanaAlertStateWithReason | PromAlertingRuleState> {
  if (isAlertStateWithReason(state)) {
    const fields = state.split(' ');
    return fields[0] as GrafanaAlertState;
  } else {
    return state;
  }
}

export enum PromRuleType {
  Alerting = 'alerting',
  Recording = 'recording',
}

export enum PromApplication {
  Cortex = 'Cortex',
  Mimir = 'Mimir',
  Prometheus = 'Prometheus',
  Thanos = 'Thanos',
}

export type RulesSourceApplication = PromApplication | 'Loki' | 'grafana';

export interface PromBuildInfoResponse {
  data: {
    application?: string;
    version: string;
    revision: string;
    features?: {
      ruler_config_api?: 'true' | 'false';
      alertmanager_config_api?: 'true' | 'false';
      query_sharding?: 'true' | 'false';
      federated_rules?: 'true' | 'false';
    };
    [key: string]: unknown;
  };
  status: 'success';
}

export interface PromApiFeatures {
  application: RulesSourceApplication;
  features: {
    rulerApiEnabled: boolean;
  };
}

export interface AlertmanagerApiFeatures {
  /**
   * Some Alertmanager implementations (Mimir) are multi-tenant systems.
   *
   * To save on compute costs, tenants are not active until they have a configuration set.
   * If there is no fallback_config_file set, Alertmanager endpoints will respond with HTTP 404
   *
   * Despite that, it is possible to create a configuration for such datasource
   * by posting a new config to the `/api/v1/alerts` endpoint
   */
  lazyConfigInit: boolean;
}

interface PromRuleDTOBase {
  health: string;
  name: string;
  query: string; // expr
  evaluationTime?: number;
  lastEvaluation?: string;
  lastError?: string;
}

interface GrafanaPromRuleDTOBase extends PromRuleDTOBase {
  uid: string;
  folderUid: string;
  isPaused: boolean;
  queriedDatasourceUIDs?: string[];
  provenance?: string;
}

export interface PromAlertingRuleDTO extends PromRuleDTOBase {
  alerts?: Array<{
    labels: Labels;
    annotations: Annotations;
    state: Exclude<PromAlertingRuleState | GrafanaAlertStateWithReason, PromAlertingRuleState.Inactive>;
    activeAt: string;
    value: string;
  }>;
  labels?: Labels;
  annotations?: Annotations;
  duration?: number; // for
  state: PromAlertingRuleState;
  type: PromRuleType.Alerting;
  notificationSettings?: GrafanaNotificationSettings;
}

export interface PromRecordingRuleDTO extends PromRuleDTOBase {
  health: string;
  name: string;
  query: string; // expr
  type: PromRuleType.Recording;
  labels?: Labels;
}

export type PromRuleDTO = PromAlertingRuleDTO | PromRecordingRuleDTO;

export interface PromRuleGroupDTO<TRule = PromRuleDTO> {
  name: string;
  file: string;
  rules: TRule[];
  interval: number;

  evaluationTime?: number; // these 2 are not in older prometheus payloads
  lastEvaluation?: string;
}

export interface GrafanaPromAlertingRuleDTO extends GrafanaPromRuleDTOBase, PromAlertingRuleDTO {
  totals: AlertInstanceTotals;
  totalsFiltered: AlertInstanceTotals;
}

export interface GrafanaPromRecordingRuleDTO extends GrafanaPromRuleDTOBase, PromRecordingRuleDTO {}

export type GrafanaPromRuleDTO = GrafanaPromAlertingRuleDTO | GrafanaPromRecordingRuleDTO;

export interface GrafanaPromRuleGroupDTO extends PromRuleGroupDTO<GrafanaPromRuleDTO> {
  folderUid: string;
}

export interface PromResponse<T> {
  status: 'success' | 'error' | ''; // mocks return empty string
  data: T;
  errorType?: string;
  error?: string;
  warnings?: string[];
}

export interface PromRulesResponse extends PromResponse<{ groups: PromRuleGroupDTO[]; groupNextToken?: string }> {}

export interface GrafanaPromRulesResponse
  extends PromResponse<{
    groups: GrafanaPromRuleGroupDTO[];
    groupNextToken?: string;
    totals?: AlertGroupTotals;
  }> {}

// Ruler rule DTOs
interface RulerRuleBaseDTO {
  expr: string;
  labels?: Labels;
}

export interface RulerRecordingRuleDTO extends RulerRuleBaseDTO {
  record: string;
}

export interface RulerAlertingRuleDTO extends RulerRuleBaseDTO {
  alert: string;
  for?: string;
  keep_firing_for?: string;
  annotations?: Annotations;
}

export enum GrafanaAlertStateDecision {
  Alerting = 'Alerting',
  NoData = 'NoData',
  KeepLast = 'KeepLast',
  OK = 'OK',
  Error = 'Error',
}

export interface AlertDataQuery extends DataQuery {
  maxDataPoints?: number;
  intervalMs?: number;
  expression?: string;
  instant?: boolean;
  range?: boolean;
}

export interface AlertQuery<T = AlertDataQuery | ExpressionQuery> {
  refId: string;
  queryType: string;
  relativeTimeRange?: RelativeTimeRange;
  datasourceUid: string;
  model: T;
}

export interface GrafanaNotificationSettings {
  receiver: string;
  group_by?: string[];
  group_wait?: string;
  group_interval?: string;
  repeat_interval?: string;
  mute_time_intervals?: string[];
  active_time_intervals?: string[];
}

export interface GrafanaEditorSettings {
  simplified_query_and_expressions_section: boolean;
  simplified_notifications_section: boolean;
}

export interface UpdatedBy {
  uid: string;
  name: string;
}
export interface PostableGrafanaRuleDefinition {
  uid?: string;
  title: string;
  condition: string;
  no_data_state?: GrafanaAlertStateDecision;
  exec_err_state?: GrafanaAlertStateDecision;
  data: AlertQuery[];
  is_paused?: boolean;
  notification_settings?: GrafanaNotificationSettings;
  metadata?: {
    editor_settings?: GrafanaEditorSettings;
  };
  record?: {
    metric: string;
    from: string;
    target_datasource_uid?: string;
  };
  intervalSeconds?: number;
  missing_series_evals_to_resolve?: number;
}
export interface GrafanaRuleDefinition extends PostableGrafanaRuleDefinition {
  id?: string;
  uid: string;
  guid?: string;
  namespace_uid: string;
  rule_group: string;
  provenance?: string;
  // TODO: For updated_by, updated, and version, fix types so these aren't optional, and
  // are not conflated with test fixtures
  updated?: string;
  updated_by?: UpdatedBy | null;
  version?: number;
}

// types for Grafana-managed recording and alerting rules
export type GrafanaAlertingRuleDefinition = Omit<GrafanaRuleDefinition, 'record'>;
export type GrafanaRecordingRuleDefinition = GrafanaRuleDefinition & {
  record: GrafanaRuleDefinition['record'];
};

export interface RulerGrafanaRuleDTO<T = GrafanaRuleDefinition> {
  grafana_alert: T;
  for?: string;
  keep_firing_for?: string;
  annotations?: Annotations;
  labels?: Labels;
}

export type TopLevelGrafanaRuleDTOField = keyof Omit<RulerGrafanaRuleDTO, 'grafana_alert'>;
export type GrafanaAlertRuleDTOField = keyof GrafanaRuleDefinition;

export type PostableRuleGrafanaRuleDTO = RulerGrafanaRuleDTO<PostableGrafanaRuleDefinition>;

export type RulerCloudRuleDTO = RulerAlertingRuleDTO | RulerRecordingRuleDTO;

export type RulerRuleDTO = RulerCloudRuleDTO | RulerGrafanaRuleDTO;
export type PostableRuleDTO = RulerCloudRuleDTO | PostableRuleGrafanaRuleDTO;

export type RulerRuleGroupDTO<R = RulerRuleDTO> = {
  name: string;
  interval?: string;
  source_tenants?: string[];
  rules: R[];
};

export type PostableRulerRuleGroupDTO = RulerRuleGroupDTO<PostableRuleDTO>;

export type RulerGrafanaRuleGroupDTO = RulerRuleGroupDTO<RulerGrafanaRuleDTO>;

export type RulerRulesConfigDTO = { [namespace: string]: RulerRuleGroupDTO[] };

export type RulerGrafanaRulesConfigDTO = { [namespace: string]: RulerGrafanaRuleGroupDTO[] };
