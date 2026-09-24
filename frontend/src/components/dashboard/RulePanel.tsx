import { useMemo, useState } from 'react';
import {
  Button,
  Card,
  Form,
  Input,
  Modal,
  Popconfirm,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Tooltip,
  Typography,
  message,
} from 'antd';
import { PlusOutlined, ReloadOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import type { AutomationRule, Device, Sensor } from '../../types/domain';
import { createRule, deleteRule, setRuleEnabled, type RuleCreatePayload } from '../../api/automation';

const { Text } = Typography;

const sensorTypeLabels: Record<string, string> = {
  temperature: '温度',
  humidity: '湿度',
  light: '光照',
  co2: 'CO₂',
  soil_moisture: '土壤湿度',
};

const actionText = (action: string) => (action === 'on' ? '开启' : '关闭');
const sideText: Record<string, string> = { high: '超上限', low: '低于下限', both: '超出上下限' };

const resultMeta: Record<string, { color: string; label: string }> = {
  success: { color: 'success', label: '成功' },
  skipped: { color: 'default', label: '跳过' },
  failed: { color: 'error', label: '失败' },
};

interface RulePanelProps {
  rules: AutomationRule[];
  sensors: Sensor[];
  devices: Device[];
  onChanged: () => void;
}

interface FormValues {
  name: string;
  sensorId: number;
  deviceId: number;
  triggerSide: 'high' | 'low' | 'both';
  triggerAction: 'on' | 'off';
  enabled: boolean;
}

export default function RulePanel({ rules, sensors, devices, onChanged }: RulePanelProps) {
  const [open, setOpen] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [toggling, setToggling] = useState<number>();
  const [removing, setRemoving] = useState<number>();
  const [form] = Form.useForm<FormValues>();

  const sensorName = useMemo(() => {
    const map = new Map<number, string>(sensors.map((s) => [s.id, s.name || sensorTypeLabels[s.type] || '传感器']));
    return (id: number, fallback?: string) => map.get(id) ?? fallback ?? `传感器#${id}`;
  }, [sensors]);
  const deviceName = useMemo(() => {
    const map = new Map<number, string>(devices.map((d) => [d.id, d.name]));
    return (id: number, fallback?: string) => map.get(id) ?? fallback ?? `设备#${id}`;
  }, [devices]);

  const openCreate = () => {
    form.resetFields();
    form.setFieldsValue({ triggerSide: 'high', triggerAction: 'on', enabled: true });
    setOpen(true);
  };

  const submit = async () => {
    const values = await form.validateFields();
    const payload: RuleCreatePayload = { ...values };
    setSubmitting(true);
    try {
      await createRule(payload);
      message.success('联动规则已创建');
      setOpen(false);
      onChanged();
    } catch {
      message.error('创建规则失败，请检查配置或重新登录');
    } finally {
      setSubmitting(false);
    }
  };

  const toggleEnabled = async (rule: AutomationRule) => {
    setToggling(rule.id);
    try {
      await setRuleEnabled(rule.id, !rule.enabled);
      message.success(rule.enabled ? '规则已停用' : '规则已启用');
      onChanged();
    } catch {
      message.error('规则状态更新失败，请重新登录');
    } finally {
      setToggling(undefined);
    }
  };

  const remove = async (rule: AutomationRule) => {
    setRemoving(rule.id);
    try {
      await deleteRule(rule.id);
      message.success('规则已移除');
      onChanged();
    } catch {
      message.error('规则移除失败，请重新登录');
    } finally {
      setRemoving(undefined);
    }
  };

  const columns: ColumnsType<AutomationRule> = [
    {
      title: '规则',
      dataIndex: 'name',
      key: 'name',
      render: (_, rule) => (
        <Space direction="vertical" size={0}>
          <Text strong>{rule.name}</Text>
          <Text type="secondary" style={{ fontSize: 12 }}>
            {sensorName(rule.sensorId, rule.sensor?.name)}
            <Text type="secondary"> · {sideText[rule.triggerSide] ?? rule.triggerSide}</Text>
          </Text>
        </Space>
      ),
    },
    {
      title: '越限 / 恢复动作',
      key: 'actions',
      render: (_, rule) => (
        <Space size={4} wrap>
          <Tag color="volcano">{actionText(rule.triggerAction)}{deviceName(rule.deviceId, rule.device?.name)}</Tag>
          <Text type="secondary">→</Text>
          <Tag color="cyan">{actionText(rule.recoveryAction)}{deviceName(rule.deviceId, rule.device?.name)}</Tag>
        </Space>
      ),
    },
    {
      title: '当前状态',
      key: 'state',
      width: 110,
      render: (_, rule) =>
        rule.enabled ? (
          <Tag color={rule.state === 'abnormal' ? 'error' : 'success'}>
            {rule.state === 'abnormal' ? '异常区间' : '正常区间'}
          </Tag>
        ) : (
          <Tag>已停用</Tag>
        ),
    },
    {
      title: '最近一次执行',
      key: 'last',
      render: (_, rule) => {
        if (!rule.lastResult) return <Text type="secondary">尚未执行</Text>;
        const meta = resultMeta[rule.lastResult];
        return (
          <Space direction="vertical" size={0}>
            <Space size={6}>
              <Tag color={meta?.color} style={{ marginInlineEnd: 0 }}>{meta?.label ?? rule.lastResult}</Tag>
              {rule.lastTriggeredAt && (
                <Text type="secondary" style={{ fontSize: 12 }}>
                  {new Date(rule.lastTriggeredAt).toLocaleString('zh-CN')}
                </Text>
              )}
            </Space>
            <Tooltip title={rule.lastMessage}>
              <Text type="secondary" ellipsis style={{ maxWidth: 260, fontSize: 12 }}>
                {rule.lastMessage}
              </Text>
            </Tooltip>
          </Space>
        );
      },
    },
    {
      title: '启停',
      key: 'enabled',
      width: 70,
      render: (_, rule) => (
        <Switch
          checked={rule.enabled}
          loading={toggling === rule.id}
          onChange={() => void toggleEnabled(rule)}
        />
      ),
    },
    {
      title: '操作',
      key: 'ops',
      width: 80,
      render: (_, rule) => (
        <Popconfirm
          title="移除该联动规则？"
          description="移除后不再自动控制设备。"
          okText="移除"
          cancelText="取消"
          okButtonProps={{ danger: true }}
          onConfirm={() => void remove(rule)}
        >
          <Button type="link" danger size="small" loading={removing === rule.id}>
            移除
          </Button>
        </Popconfirm>
      ),
    },
  ];

  return (
    <Card
      title="阈值联动规则"
      extra={
        <Space>
          <Button size="small" icon={<ReloadOutlined />} onClick={onChanged}>刷新</Button>
          <Button size="small" type="primary" icon={<PlusOutlined />} onClick={openCreate}>
            新建规则
          </Button>
        </Space>
      }
    >
      <Table<AutomationRule>
        rowKey="id"
        size="small"
        columns={columns}
        dataSource={rules}
        locale={{ emptyText: '暂无联动规则，点击“新建规则”自动处理超限' }}
        pagination={false}
      />
      <Modal
        title="新建阈值联动规则"
        open={open}
        onCancel={() => setOpen(false)}
        onOk={() => void submit()}
        confirmLoading={submitting}
        okText="创建"
        cancelText="取消"
        destroyOnClose
      >
        <Form form={form} layout="vertical" initialValues={{ triggerSide: 'high', triggerAction: 'on', enabled: true }}>
          <Form.Item name="name" label="规则名称" rules={[{ required: true, message: '请输入规则名称', min: 2 }]}>
            <Input placeholder="例如：温度超限开启风机" maxLength={100} />
          </Form.Item>
          <Form.Item name="sensorId" label="监测传感器" rules={[{ required: true, message: '请选择传感器' }]}>
            <Select
              placeholder="选择传感器"
              options={sensors.map((s) => ({
                value: s.id,
                label: `${s.name || sensorTypeLabels[s.type]}（${sensorTypeLabels[s.type] ?? s.type}）`,
              }))}
            />
          </Form.Item>
          <Space size={12} style={{ display: 'flex' }}>
            <Form.Item
              name="triggerSide"
              label="触发条件"
              style={{ flex: 1, marginBottom: 12 }}
              rules={[{ required: true }]}
            >
              <Select
                options={[
                  { value: 'high', label: '超出上限' },
                  { value: 'low', label: '低于下限' },
                  { value: 'both', label: '超出上限或下限' },
                ]}
              />
            </Form.Item>
            <Form.Item
              name="triggerAction"
              label="越限时动作"
              style={{ flex: 1, marginBottom: 12 }}
              tooltip="恢复到正常范围后自动执行相反动作"
              rules={[{ required: true }]}
            >
              <Select
                options={[
                  { value: 'on', label: '开启设备' },
                  { value: 'off', label: '关闭设备' },
                ]}
              />
            </Form.Item>
          </Space>
          <Form.Item name="deviceId" label="控制设备" rules={[{ required: true, message: '请选择设备' }]}>
            <Select
              placeholder="选择设备（仅限同一温室）"
              options={devices.map((d) => ({ value: d.id, label: `${d.name}（${d.type}）` }))}
            />
          </Form.Item>
          <Form.Item name="enabled" label="创建后立即启用" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Text type="secondary" style={{ fontSize: 12 }}>
            读数越限时执行上述动作；恢复到正常范围后自动执行相反动作。一次异常只处理一次。
          </Text>
        </Form>
      </Modal>
    </Card>
  );
}
