/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import React, { useEffect, useMemo, useState } from 'react';
import {
  Banner,
  Button,
  Card,
  Col,
  Form,
  Input,
  Row,
  Space,
  Switch,
  Typography,
} from '@douyinfe/semi-ui';
import {
  API,
  showError,
  showSuccess,
  showWarning,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';

const { Text, Title } = Typography;

const DEFAULT_TIMEOUT_STATUS = 503;
const DEFAULT_TIMEOUT_MESSAGE = '资源繁忙，请稍后尝试';

function parseJSONSafe(str, fallback) {
  if (!str || !String(str).trim()) return fallback;
  try {
    return JSON.parse(str);
  } catch {
    return fallback;
  }
}

function parseGroupNames(groupRatio) {
  const ratioMap = parseJSONSafe(groupRatio, {});
  return Object.keys(ratioMap).sort((a, b) => a.localeCompare(b));
}

function buildRows(groupNames, strategySettings) {
  const parsed = parseJSONSafe(strategySettings, {});
  return groupNames.map((group) => {
    const current = parsed[group] || {};
    return {
      group,
      enabled: !!current.enabled,
      stream_retry_first_byte_budget_seconds:
        typeof current.stream_retry_first_byte_budget_seconds === 'number'
          ? String(current.stream_retry_first_byte_budget_seconds)
          : '',
      retry_times:
        typeof current.retry_times === 'number'
          ? String(current.retry_times)
          : '',
      timeout_http_status:
        typeof current.timeout_http_status === 'number'
          ? String(current.timeout_http_status)
          : '',
      timeout_error_message: current.timeout_error_message || '',
      existsInOption: !!parsed[group],
    };
  });
}

function serializeRows(rows) {
  const payload = {};
  rows.forEach((row) => {
    const item = {
      enabled: !!row.enabled,
    };
    if (row.stream_retry_first_byte_budget_seconds !== '') {
      item.stream_retry_first_byte_budget_seconds = Number(
        row.stream_retry_first_byte_budget_seconds,
      );
    }
    if (row.retry_times !== '') {
      item.retry_times = Number(row.retry_times);
    }
    if (row.timeout_http_status !== '') {
      item.timeout_http_status = Number(row.timeout_http_status);
    }
    if ((row.timeout_error_message || '').trim()) {
      item.timeout_error_message = row.timeout_error_message.trim();
    }

    const hasCustomField =
      item.stream_retry_first_byte_budget_seconds !== undefined ||
      item.retry_times !== undefined ||
      item.timeout_http_status !== undefined ||
      !!item.timeout_error_message;

    if (row.enabled || hasCustomField || row.existsInOption) {
      payload[row.group] = item;
    }
  });
  return JSON.stringify(payload, null, 2);
}

function validateRows(rows, t) {
  for (const row of rows) {
    if (!row.enabled) continue;
    if (
      row.stream_retry_first_byte_budget_seconds !== '' &&
      Number(row.stream_retry_first_byte_budget_seconds) < 1
    ) {
      return t('分组 {{group}} 的流式首包重试预算至少为 1 秒', {
        group: row.group,
      });
    }
    if (row.retry_times !== '' && Number(row.retry_times) < 0) {
      return t('分组 {{group}} 的重试次数不能小于 0', {
        group: row.group,
      });
    }
    if (row.timeout_http_status !== '') {
      const status = Number(row.timeout_http_status);
      if (status < 100 || status > 599) {
        return t('分组 {{group}} 的超时状态码必须在 100 到 599 之间', {
          group: row.group,
        });
      }
    }
  }
  return '';
}

function buildRowsSignature(rows) {
  return JSON.stringify(
    rows.map((row) => ({
      group: row.group,
      enabled: row.enabled,
      stream_retry_first_byte_budget_seconds:
        row.stream_retry_first_byte_budget_seconds,
      retry_times: row.retry_times,
      timeout_http_status: row.timeout_http_status,
      timeout_error_message: row.timeout_error_message,
      existsInOption: row.existsInOption,
    })),
  );
}

export default function SettingsStrategy(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [rows, setRows] = useState([]);
  const [rowsSnapshot, setRowsSnapshot] = useState([]);

  const groupNames = useMemo(
    () => parseGroupNames(props.options?.GroupRatio || ''),
    [props.options],
  );

  useEffect(() => {
    const nextRows = buildRows(
      groupNames,
      props.options?.group_strategy_settings || '',
    );
    setRows(nextRows);
    setRowsSnapshot(structuredClone(nextRows));
  }, [groupNames, props.options]);

  const updateRow = (group, field, value) => {
    setRows((prev) =>
      prev.map((row) => (row.group === group ? { ...row, [field]: value } : row)),
    );
  };

  const hasChanges =
    buildRowsSignature(rows) !== buildRowsSignature(rowsSnapshot);

  const onSubmit = async () => {
    if (!hasChanges) {
      return showWarning(t('你似乎并没有修改什么'));
    }

    const validationError = validateRows(rows, t);
    if (validationError) {
      return showError(validationError);
    }

    setLoading(true);
    try {
      const value = serializeRows(rows);
      const res = await API.put('/api/option/', {
        key: 'group_strategy_settings',
        value,
      });
      const { success, message } = res.data;
      if (!success) {
        return showError(message);
      }
      showSuccess(t('保存成功'));
      await props.refresh();
    } catch (error) {
      showError(t('保存失败，请重试'));
    } finally {
      setLoading(false);
    }
  };

  const onReset = () => {
    setRows(structuredClone(rowsSnapshot));
  };

  return (
    <Card>
      <Form.Section
        text={t('策略设置')}
        extraText={t('按分组覆盖重试次数、流式首包等待预算以及预算耗尽时的回退响应')}
      >
        <Space vertical align='start' style={{ width: '100%' }} spacing='small'>
          <div
            style={{
              width: '100%',
              border: '1px solid var(--semi-color-border)',
              borderRadius: 8,
              padding: 12,
              background: 'var(--semi-color-fill-0)',
            }}
          >
            <Row gutter={[16, 8]}>
              <Col xs={24} md={8}>
                <Text strong>{t('优先级')}</Text>
                <Text
                  type='secondary'
                  size='small'
                  style={{ display: 'block', marginTop: 4 }}
                >
                  {t(
                    '命中分组后，重试次数优先使用分组值；未命中时继续沿用全局 RetryTimes。',
                  )}
                </Text>
              </Col>
              <Col xs={24} md={8}>
                <Text strong>{t('预算口径')}</Text>
                <Text
                  type='secondary'
                  size='small'
                  style={{ display: 'block', marginTop: 4 }}
                >
                  {t(
                    '只统计每次重试等待流式首包的耗时之和，不统计完整输出时长。',
                  )}
                </Text>
              </Col>
              <Col xs={24} md={8}>
                <Text strong>{t('回退响应')}</Text>
                <Text
                  type='secondary'
                  size='small'
                  style={{ display: 'block', marginTop: 4 }}
                >
                  {t(
                    '上游没有更明确错误时，回退到当前分组的状态码与文案；默认 503 / 资源繁忙，请稍后尝试。',
                  )}
                </Text>
              </Col>
            </Row>
          </div>

          <div
            style={{
              width: '100%',
              display: 'flex',
              alignItems: 'center',
            }}
          >
            <Text type='secondary' size='small'>
              {t(
                '这里的重试次数仅指 New API 在不同渠道之间继续尝试的次数，首次请求不计入，也不是 AWS SDK 自身的重试。',
              )}
            </Text>
          </div>

          {groupNames.length === 0 ? (
            <Banner
              type='info'
              fullMode={false}
              title={t('暂无可配置分组')}
              description={t(
                '请先到“分组与模型定价设置”中配置 GroupRatio，例如 default、vip，随后再回到这里设置分组策略。',
              )}
            />
          ) : (
            rows.map((row) => {
              const usingFallback = !row.enabled;
              return (
                <Card
                  key={row.group}
                  style={{
                    width: '100%',
                    border: '1px solid var(--semi-color-border)',
                  }}
                  bodyStyle={{ padding: 12 }}
                >
                  <Space vertical align='start' style={{ width: '100%' }}>
                    <div
                      style={{
                        width: '100%',
                        display: 'flex',
                        justifyContent: 'space-between',
                        gap: 12,
                        alignItems: 'center',
                        flexWrap: 'wrap',
                      }}
                    >
                      <div>
                        <Space spacing='tight' align='center'>
                          <Title heading={6} style={{ margin: 0 }}>
                            {row.group}
                          </Title>
                          <Text
                            style={{
                              fontSize: 12,
                              padding: '2px 8px',
                              borderRadius: 999,
                              background: usingFallback
                                ? 'var(--semi-color-fill-0)'
                                : 'rgba(var(--semi-blue-5), 0.12)',
                              color: usingFallback
                                ? 'var(--semi-color-text-2)'
                                : 'var(--semi-color-primary)',
                            }}
                          >
                            {usingFallback ? t('跟随全局') : t('独立策略')}
                          </Text>
                        </Space>
                        <Text
                          type='secondary'
                          size='small'
                          style={{ display: 'block', marginTop: 4 }}
                        >
                          {usingFallback
                            ? t('未启用时继续使用全局失败重试次数和现有超时行为')
                            : t('启用后覆盖全局失败重试次数，并增加该分组的流式预算与回退响应')}
                        </Text>
                      </div>
                      <div style={{ minWidth: 88, textAlign: 'right' }}>
                        <Text
                          type='secondary'
                          size='small'
                          style={{ display: 'block', marginBottom: 4 }}
                        >
                          {t('启用分组策略')}
                        </Text>
                        <Switch
                          checked={row.enabled}
                          checkedText='开'
                          uncheckedText='关'
                          onChange={(checked) =>
                            updateRow(row.group, 'enabled', checked)
                          }
                        />
                      </div>
                    </div>

                    <Row gutter={[12, 8]} style={{ width: '100%' }}>
                      <Col xs={24} sm={12} md={6}>
                        <Form.Slot
                          label={t('流式首包重试预算（秒）')}
                          extraText={t('留空则不额外限制')}
                        >
                          <Input
                            value={row.stream_retry_first_byte_budget_seconds}
                            placeholder={t('例如 15')}
                            disabled={!row.enabled}
                            onChange={(value) =>
                              updateRow(
                                row.group,
                                'stream_retry_first_byte_budget_seconds',
                                value,
                              )
                            }
                          />
                        </Form.Slot>
                      </Col>

                      <Col xs={24} sm={12} md={6}>
                        <Form.Slot
                          label={t('重试次数')}
                          extraText={t('首次请求不计入')}
                        >
                          <Input
                            value={row.retry_times}
                            placeholder={String(props.options?.RetryTimes ?? 0)}
                            disabled={!row.enabled}
                            onChange={(value) =>
                              updateRow(row.group, 'retry_times', value)
                            }
                          />
                        </Form.Slot>
                      </Col>

                      <Col xs={24} sm={12} md={6}>
                        <Form.Slot
                          label={t('超时 HTTP 状态码')}
                          extraText={t('默认 503')}
                        >
                          <Input
                            value={row.timeout_http_status}
                            placeholder={String(DEFAULT_TIMEOUT_STATUS)}
                            disabled={!row.enabled}
                            onChange={(value) =>
                              updateRow(row.group, 'timeout_http_status', value)
                            }
                          />
                        </Form.Slot>
                      </Col>

                      <Col xs={24} sm={12} md={6}>
                        <Form.Slot
                          label={t('超时错误文案')}
                          extraText={t('默认繁忙提示')}
                        >
                          <Input
                            value={row.timeout_error_message}
                            placeholder={DEFAULT_TIMEOUT_MESSAGE}
                            disabled={!row.enabled}
                            onChange={(value) =>
                              updateRow(
                                row.group,
                                'timeout_error_message',
                                value,
                              )
                            }
                          />
                        </Form.Slot>
                      </Col>
                    </Row>

                    <div
                      style={{
                        width: '100%',
                        display: 'flex',
                        justifyContent: 'space-between',
                        gap: 12,
                        flexWrap: 'wrap',
                        paddingTop: 4,
                      }}
                    >
                      <Text type='secondary' size='small'>
                        {row.retry_times === ''
                          ? t('当前回退到全局重试次数：{{count}}', {
                              count: props.options?.RetryTimes ?? 0,
                            })
                          : t('当前允许首次请求之后再重试 {{count}} 次', {
                              count: row.retry_times,
                            })}
                      </Text>
                      <Text type='secondary' size='small'>
                        {row.timeout_error_message?.trim()
                          ? t('当前回退文案：{{message}}', {
                              message: row.timeout_error_message.trim(),
                            })
                          : t('当前默认回退文案：{{message}}', {
                              message: DEFAULT_TIMEOUT_MESSAGE,
                            })}
                      </Text>
                    </div>
                  </Space>
                </Card>
              );
            })
          )}

          <div
            style={{
              width: '100%',
              display: 'flex',
              justifyContent: 'flex-start',
              gap: '12px',
              alignItems: 'center',
              paddingTop: '8px',
              borderTop: '1px solid var(--semi-color-border)',
              marginTop: 4,
            }}
          >
            <Button
              size='default'
              type='tertiary'
              onClick={onReset}
              disabled={loading || !hasChanges}
              style={{
                borderRadius: '6px',
                fontWeight: '500',
              }}
            >
              {t('重置为默认')}
            </Button>
            <Button
              size='default'
              type='primary'
              onClick={onSubmit}
              loading={loading}
              style={{
                borderRadius: '6px',
                fontWeight: '500',
                minWidth: '100px',
              }}
            >
              {t('保存设置')}
            </Button>
          </div>
        </Space>
      </Form.Section>
    </Card>
  );
}
