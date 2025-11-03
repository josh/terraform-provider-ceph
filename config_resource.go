package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceSchema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
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
	Section types.String `tfsdk:"section"`
	Config  types.Map    `tfsdk:"config"`
}

func (r *ConfigResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_config"
}

func (r *ConfigResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceSchema.Schema{
		MarkdownDescription: "Manages Ceph cluster configuration values for a specific section (e.g., global, mon, osd, osd.0).",
		Attributes: map[string]resourceSchema.Attribute{
			"section": resourceSchema.StringAttribute{
				MarkdownDescription: "The section to apply configurations to (e.g., 'global', 'mon', 'osd', 'osd.0'). This determines which daemon(s) the configuration applies to.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"config": resourceSchema.MapAttribute{
				MarkdownDescription: "Map of configuration names to values for the specified section.",
				Required:            true,
				ElementType:         types.StringType,
				Validators: []validator.Map{
					NoMgrPrefixKeys(),
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

	section := data.Section.ValueString()

	var configs map[string]string
	resp.Diagnostics.Append(data.Config.ElementsAs(ctx, &configs, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var createdConfigs []string

	for name, value := range configs {
		err := r.client.ClusterUpdateConf(ctx, name, section, value)
		if err != nil {
			resp.Diagnostics.AddError(
				"API Request Error",
				fmt.Sprintf("Unable to create cluster configuration %s/%s: %s", section, name, err),
			)

			for _, createdName := range createdConfigs {
				rollbackErr := r.client.ClusterDeleteConf(ctx, createdName, section)
				if rollbackErr != nil {
					resp.Diagnostics.AddError(
						"Rollback Failed",
						fmt.Sprintf("Failed to rollback configuration %s/%s: %s. Cluster may be in an inconsistent state. Manual intervention may be required.", section, createdName, rollbackErr),
					)
					return
				}
			}
			return
		}

		createdConfigs = append(createdConfigs, name)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ConfigResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	section := data.Section.ValueString()

	var configs map[string]string
	resp.Diagnostics.Append(data.Config.ElementsAs(ctx, &configs, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updatedConfigs := make(map[string]string)

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
				updatedConfigs[name] = v.Value
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

	if len(updatedConfigs) == 0 {
		resp.State.RemoveResource(ctx)
		return
	}

	configValue, diags := types.MapValueFrom(ctx, types.StringType, updatedConfigs)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	data.Config = configValue
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var oldData, newData ConfigResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &oldData)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &newData)...)

	if resp.Diagnostics.HasError() {
		return
	}

	section := newData.Section.ValueString()

	var oldConfigs, newConfigs map[string]string
	resp.Diagnostics.Append(oldData.Config.ElementsAs(ctx, &oldConfigs, false)...)
	resp.Diagnostics.Append(newData.Config.ElementsAs(ctx, &newConfigs, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	for name, newValue := range newConfigs {
		oldValue, exists := oldConfigs[name]

		if !exists {
			err := r.client.ClusterUpdateConf(ctx, name, section, newValue)
			if err != nil {
				resp.Diagnostics.AddError(
					"API Request Error",
					fmt.Sprintf("Unable to create cluster configuration %s/%s: %s", section, name, err),
				)
				return
			}
		} else if oldValue != newValue {
			err := r.client.ClusterUpdateConf(ctx, name, section, newValue)
			if err != nil {
				resp.Diagnostics.AddError(
					"API Request Error",
					fmt.Sprintf("Unable to update cluster configuration %s/%s: %s", section, name, err),
				)
				return
			}
		}
	}

	for name := range oldConfigs {
		if _, exists := newConfigs[name]; !exists {
			err := r.client.ClusterDeleteConf(ctx, name, section)
			if err != nil {
				resp.Diagnostics.AddError(
					"API Request Error",
					fmt.Sprintf("Unable to delete cluster configuration %s/%s: %s. Update operation failed.", section, name, err),
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

	section := data.Section.ValueString()

	var configs map[string]string
	resp.Diagnostics.Append(data.Config.ElementsAs(ctx, &configs, false)...)
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

func (r *ConfigResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	section := strings.TrimSpace(req.ID)

	if section == "" {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			"Import ID cannot be empty. Expected format: section name (e.g., 'global', 'mon', 'osd', 'osd.0')",
		)
		return
	}

	allConfigs, err := r.client.ClusterListConf(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to list cluster configurations during import: %s", err),
		)
		return
	}

	importedConfigs := make(map[string]string)

	for _, config := range allConfigs {
		if len(config.Value) == 0 {
			continue
		}

		if strings.HasPrefix(config.Name, "mgr/") {
			continue
		}

		for _, v := range config.Value {
			if v.Section == section {
				importedConfigs[config.Name] = v.Value
				break
			}
		}
	}

	if len(importedConfigs) == 0 {
		resp.Diagnostics.AddError(
			"No Configurations Found",
			fmt.Sprintf("No non-default configurations found for section '%s'. The section may only have default values set, or the section name may be incorrect.", section),
		)
		return
	}

	configValue, diags := types.MapValueFrom(ctx, types.StringType, importedConfigs)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	data := ConfigResourceModel{
		Section: types.StringValue(section),
		Config:  configValue,
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
