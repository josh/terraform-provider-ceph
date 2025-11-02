package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceSchema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &ConfigResource{}
	_ resource.ResourceWithImportState = &ConfigResource{}
)

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
			if strings.HasPrefix(name, "mgr/") {
				resp.Diagnostics.AddError(
					"Invalid Configuration Name",
					fmt.Sprintf("Configuration '%s' cannot be managed via ceph_config. Use ceph_mgr_module_config instead.", name),
				)
				return
			}

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
			if strings.HasPrefix(name, "mgr/") {
				resp.Diagnostics.AddError(
					"Invalid Configuration Name",
					fmt.Sprintf("Configuration '%s' cannot be managed via ceph_config. Use ceph_mgr_module_config instead.", name),
				)
				return
			}

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

func (r *ConfigResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	var importedSections map[string]map[string]string

	if req.ID == "" || req.ID == "*" || req.ID == "id-attribute-not-set" {
		allConfigs, err := r.client.ClusterListConf(ctx)
		if err != nil {
			resp.Diagnostics.AddError(
				"API Request Error",
				fmt.Sprintf("Unable to list cluster configurations during bulk import: %s", err),
			)
			return
		}

		importedSections = make(map[string]map[string]string)

		for _, config := range allConfigs {
			if len(config.Value) == 0 {
				continue
			}

			if strings.HasPrefix(config.Name, "mgr/") {
				continue
			}

			for _, v := range config.Value {
				if importedSections[v.Section] == nil {
					importedSections[v.Section] = make(map[string]string)
				}
				importedSections[v.Section][config.Name] = v.Value
			}
		}

		if len(importedSections) == 0 {
			resp.Diagnostics.AddError(
				"No Configurations Found",
				"No non-default configurations found to import. The cluster may only have default values set.",
			)
			return
		}
	} else {
		importPairs := strings.Split(req.ID, ",")
		if len(importPairs) == 0 {
			resp.Diagnostics.AddError(
				"Invalid Import ID",
				"Import ID cannot be empty. Expected format: 'section.key' or 'section1.key1,section2.key2'",
			)
			return
		}

		configsBySection := make(map[string]map[string]string)

		for _, pair := range importPairs {
			pair = strings.TrimSpace(pair)
			parts := strings.Split(pair, ".")

			if len(parts) != 2 {
				resp.Diagnostics.AddError(
					"Invalid Import ID Format",
					fmt.Sprintf("Expected format 'section.key', got: %s. Full import ID: %s. Use empty string or '*' for bulk import.", pair, req.ID),
				)
				return
			}

			section := strings.TrimSpace(parts[0])
			name := strings.TrimSpace(parts[1])

			if section == "" || name == "" {
				resp.Diagnostics.AddError(
					"Invalid Import ID",
					fmt.Sprintf("Section and key cannot be empty in: %s", pair),
				)
				return
			}

			if configsBySection[section] == nil {
				configsBySection[section] = make(map[string]string)
			}

			if _, exists := configsBySection[section][name]; exists {
				resp.Diagnostics.AddError(
					"Duplicate Import Entry",
					fmt.Sprintf("Config %s/%s appears multiple times in import ID", section, name),
				)
				return
			}

			configsBySection[section][name] = ""
		}

		importedSections = make(map[string]map[string]string)

		for section, configs := range configsBySection {
			for name := range configs {
				apiConfig, err := r.client.ClusterGetConf(ctx, name)
				if err != nil {
					resp.Diagnostics.AddError(
						"API Request Error",
						fmt.Sprintf("Unable to read cluster configuration %s/%s during import: %s", section, name, err),
					)
					return
				}

				found := false
				for _, v := range apiConfig.Value {
					if v.Section == section {
						if importedSections[section] == nil {
							importedSections[section] = make(map[string]string)
						}
						importedSections[section][name] = v.Value
						found = true
						break
					}
				}

				if !found {
					resp.Diagnostics.AddError(
						"Configuration Not Found",
						fmt.Sprintf("Configuration %s/%s does not exist in the cluster or has no value set for section %s", section, name, section),
					)
					return
				}
			}
		}
	}

	sectionMaps := make(map[string]types.Map)
	for section, configs := range importedSections {
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

	data := ConfigResourceModel{
		Configs: configsValue,
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
