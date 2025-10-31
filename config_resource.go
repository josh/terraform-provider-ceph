package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceSchema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &ConfigResource{}

func newConfigResource() resource.Resource {
	return &ConfigResource{}
}

type ConfigResource struct {
	client *CephAPIClient
}

type ConfigResourceModel struct {
	Configs types.Map `tfsdk:"configs"`
}

type configKey struct {
	section string
	name    string
}

func (r *ConfigResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_config"
}

func (r *ConfigResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceSchema.Schema{
		MarkdownDescription: "Manages Ceph cluster configuration values. This resource can manage one or more configuration settings across different sections (e.g., global, mon, osd, osd.0).",
		Attributes: map[string]resourceSchema.Attribute{
			"configs": resourceSchema.MapAttribute{
				MarkdownDescription: "Map of sections to configuration key-value pairs. Each section (e.g., 'global', 'osd') contains a map of configuration names to values.",
				Required:            true,
				ElementType: types.MapType{
					ElemType: types.StringType,
				},
			},
		},
	}
}

func (r *ConfigResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*CephAPIClient)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *CephAPIClient, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = client
}

func (r *ConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ConfigResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	var sections map[string]types.Map
	resp.Diagnostics.Append(data.Configs.ElementsAs(ctx, &sections, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var createdConfigs []configKey

	for section, configMap := range sections {
		var configs map[string]string
		resp.Diagnostics.Append(configMap.ElementsAs(ctx, &configs, false)...)
		if resp.Diagnostics.HasError() {
			return
		}

		for name, value := range configs {
			err := r.client.ClusterUpdateConf(ctx, name, section, value)
			if err != nil {
				resp.Diagnostics.AddError(
					"API Request Error",
					fmt.Sprintf("Unable to create cluster configuration %s/%s: %s", section, name, err),
				)

				for _, created := range createdConfigs {
					rollbackErr := r.client.ClusterDeleteConf(ctx, created.name, created.section)
					if rollbackErr != nil {
						resp.Diagnostics.AddError(
							"Rollback Failed",
							fmt.Sprintf("Failed to rollback configuration %s/%s: %s. Cluster may be in an inconsistent state. Manual intervention may be required.", created.section, created.name, rollbackErr),
						)
						return
					}
				}
				return
			}

			createdConfigs = append(createdConfigs, configKey{section: section, name: name})
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ConfigResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	var sections map[string]types.Map
	resp.Diagnostics.Append(data.Configs.ElementsAs(ctx, &sections, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updatedSections := make(map[string]map[string]string)

	for section, configMap := range sections {
		var configs map[string]string
		resp.Diagnostics.Append(configMap.ElementsAs(ctx, &configs, false)...)
		if resp.Diagnostics.HasError() {
			return
		}

		for name := range configs {
			apiConfig, err := r.client.ClusterGetConf(ctx, name)
			if err != nil {
				resp.Diagnostics.AddError(
					"API Request Error",
					fmt.Sprintf("Unable to read cluster configuration %s/%s: %s", section, name, err),
				)
				return
			}

			found := false
			for _, v := range apiConfig.Value {
				if v.Section == section {
					if updatedSections[section] == nil {
						updatedSections[section] = make(map[string]string)
					}
					updatedSections[section][name] = v.Value
					found = true
					break
				}
			}

			if !found {
				resp.Diagnostics.AddWarning(
					"Configuration Drift Detected",
					fmt.Sprintf("Configuration %s/%s no longer exists in cluster. Removing from state.", section, name),
				)
			}
		}
	}

	if len(updatedSections) == 0 {
		resp.State.RemoveResource(ctx)
		return
	}

	sectionMaps := make(map[string]types.Map)
	for section, configs := range updatedSections {
		configMap, diags := types.MapValueFrom(ctx, types.StringType, configs)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		sectionMaps[section] = configMap
	}

	configsValue, diags := types.MapValueFrom(ctx, types.MapType{ElemType: types.StringType}, sectionMaps)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	data.Configs = configsValue
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var oldData, newData ConfigResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &oldData)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &newData)...)

	if resp.Diagnostics.HasError() {
		return
	}

	var oldSections, newSections map[string]types.Map
	resp.Diagnostics.Append(oldData.Configs.ElementsAs(ctx, &oldSections, false)...)
	resp.Diagnostics.Append(newData.Configs.ElementsAs(ctx, &newSections, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	oldConfigMap := make(map[configKey]string)
	for section, configMap := range oldSections {
		var configs map[string]string
		resp.Diagnostics.Append(configMap.ElementsAs(ctx, &configs, false)...)
		if resp.Diagnostics.HasError() {
			return
		}

		for name, value := range configs {
			key := configKey{section: section, name: name}
			oldConfigMap[key] = value
		}
	}

	newConfigMap := make(map[configKey]string)
	for section, configMap := range newSections {
		var configs map[string]string
		resp.Diagnostics.Append(configMap.ElementsAs(ctx, &configs, false)...)
		if resp.Diagnostics.HasError() {
			return
		}

		for name, value := range configs {
			key := configKey{section: section, name: name}
			newConfigMap[key] = value
		}
	}

	for key, newValue := range newConfigMap {
		oldValue, exists := oldConfigMap[key]

		if !exists {
			err := r.client.ClusterUpdateConf(ctx, key.name, key.section, newValue)
			if err != nil {
				resp.Diagnostics.AddError(
					"API Request Error",
					fmt.Sprintf("Unable to create cluster configuration %s/%s: %s", key.section, key.name, err),
				)
				return
			}
		} else if oldValue != newValue {
			err := r.client.ClusterUpdateConf(ctx, key.name, key.section, newValue)
			if err != nil {
				resp.Diagnostics.AddError(
					"API Request Error",
					fmt.Sprintf("Unable to update cluster configuration %s/%s: %s", key.section, key.name, err),
				)
				return
			}
		}
	}

	for key := range oldConfigMap {
		if _, exists := newConfigMap[key]; !exists {
			err := r.client.ClusterDeleteConf(ctx, key.name, key.section)
			if err != nil {
				resp.Diagnostics.AddError(
					"API Request Error",
					fmt.Sprintf("Unable to delete cluster configuration %s/%s: %s. Update operation failed.", key.section, key.name, err),
				)
				return
			}
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &newData)...)
}

func (r *ConfigResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ConfigResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	var sections map[string]types.Map
	resp.Diagnostics.Append(data.Configs.ElementsAs(ctx, &sections, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	for section, configMap := range sections {
		var configs map[string]string
		resp.Diagnostics.Append(configMap.ElementsAs(ctx, &configs, false)...)
		if resp.Diagnostics.HasError() {
			return
		}

		for name := range configs {
			err := r.client.ClusterDeleteConf(ctx, name, section)
			if err != nil {
				resp.Diagnostics.AddWarning(
					"API Request Warning",
					fmt.Sprintf("Unable to delete cluster configuration %s/%s: %s. Continuing with remaining deletions.", section, name, err),
				)
			}
		}
	}
}
