package main

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceSchema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &MgrModuleConfigResource{}
	_ resource.ResourceWithImportState = &MgrModuleConfigResource{}
)

func newMgrModuleConfigResource() resource.Resource {
	return &MgrModuleConfigResource{}
}

type MgrModuleConfigResource struct {
	client *CephAPIClient
}

type MgrModuleConfigResourceModel struct {
	ModuleName types.String `tfsdk:"module_name"`
	Configs    types.Map    `tfsdk:"configs"`
	ID         types.String `tfsdk:"id"`
}

func formatMgrModuleConfigValue(val interface{}) (string, error) {
	switch v := val.(type) {
	case float64:
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v)), nil
		}
		return strconv.FormatFloat(v, 'g', -1, 64), nil
	case float32:
		if v == float32(int32(v)) {
			return fmt.Sprintf("%d", int32(v)), nil
		}
		return strconv.FormatFloat(float64(v), 'g', -1, 32), nil
	case int, int8, int16, int32, int64:
		return fmt.Sprintf("%d", v), nil
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", v), nil
	case bool:
		return fmt.Sprintf("%t", v), nil
	case string:
		return v, nil
	default:
		return "", fmt.Errorf("unsupported config value type: %T", v)
	}
}

func (r *MgrModuleConfigResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mgr_module_config"
}

func (r *MgrModuleConfigResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceSchema.Schema{
		MarkdownDescription: "Manages configuration settings for a Ceph MGR module. " +
			"This resource allows you to configure MGR modules like dashboard, telemetry, or prometheus. " +
			"Note: Some configuration changes may require a module restart to take effect.",
		Attributes: map[string]resourceSchema.Attribute{
			"module_name": resourceSchema.StringAttribute{
				MarkdownDescription: "The name of the MGR module to configure (e.g., 'dashboard', 'telemetry', 'prometheus')",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"configs": resourceSchema.MapAttribute{
				MarkdownDescription: "Map of configuration option names to their values. " +
					"Values should be provided as strings. The provider will convert them to appropriate types (bool, int, string) when sending to the API.",
				Required:    true,
				ElementType: types.StringType,
			},
			"id": resourceSchema.StringAttribute{
				MarkdownDescription: "Identifier for this resource (set to module_name)",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *MgrModuleConfigResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *MgrModuleConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data MgrModuleConfigResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	moduleName := data.ModuleName.ValueString()

	var configsMap map[string]string
	resp.Diagnostics.Append(data.Configs.ElementsAs(ctx, &configsMap, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiConfigs := make(CephAPIMgrModuleConfig)
	for key, value := range configsMap {
		apiConfigs[key] = value
	}

	err := r.client.MgrSetModuleConfig(ctx, moduleName, apiConfigs)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to create MGR module config for '%s': %s", moduleName, err),
		)
		return
	}

	data.ID = types.StringValue(moduleName)

	readConfigs, err := r.client.MgrGetModuleConfig(ctx, moduleName)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read back MGR module config for '%s': %s", moduleName, err),
		)
		return
	}

	stringConfigs := make(map[string]string)
	for key := range configsMap {
		if val, ok := readConfigs[key]; ok {
			formattedVal, err := formatMgrModuleConfigValue(val)
			if err != nil {
				resp.Diagnostics.AddError(
					"Configuration Value Formatting Error",
					fmt.Sprintf("Unable to format config value for key '%s': %s", key, err),
				)
				return
			}
			stringConfigs[key] = formattedVal
		}
	}

	configsValue, diags := types.MapValueFrom(ctx, types.StringType, stringConfigs)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	data.Configs = configsValue

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *MgrModuleConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data MgrModuleConfigResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	moduleName := data.ModuleName.ValueString()

	readConfigs, err := r.client.MgrGetModuleConfig(ctx, moduleName)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read MGR module config for '%s': %s", moduleName, err),
		)
		return
	}

	var currentConfigsMap map[string]string
	resp.Diagnostics.Append(data.Configs.ElementsAs(ctx, &currentConfigsMap, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	stringConfigs := make(map[string]string)
	for key := range currentConfigsMap {
		if val, ok := readConfigs[key]; ok {
			formattedVal, err := formatMgrModuleConfigValue(val)
			if err != nil {
				resp.Diagnostics.AddError(
					"Configuration Value Formatting Error",
					fmt.Sprintf("Unable to format config value for key '%s': %s", key, err),
				)
				return
			}
			stringConfigs[key] = formattedVal
		}
	}

	configsValue, diags := types.MapValueFrom(ctx, types.StringType, stringConfigs)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	data.Configs = configsValue

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *MgrModuleConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data MgrModuleConfigResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	moduleName := data.ModuleName.ValueString()

	var newConfigsMap map[string]string
	resp.Diagnostics.Append(data.Configs.ElementsAs(ctx, &newConfigsMap, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiConfigs := make(CephAPIMgrModuleConfig)
	for key, value := range newConfigsMap {
		apiConfigs[key] = value
	}

	err := r.client.MgrSetModuleConfig(ctx, moduleName, apiConfigs)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to update MGR module config for '%s': %s", moduleName, err),
		)
		return
	}

	readConfigs, err := r.client.MgrGetModuleConfig(ctx, moduleName)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read back MGR module config for '%s': %s", moduleName, err),
		)
		return
	}

	stringConfigs := make(map[string]string)
	for key := range newConfigsMap {
		if val, ok := readConfigs[key]; ok {
			formattedVal, err := formatMgrModuleConfigValue(val)
			if err != nil {
				resp.Diagnostics.AddError(
					"Configuration Value Formatting Error",
					fmt.Sprintf("Unable to format config value for key '%s': %s", key, err),
				)
				return
			}
			stringConfigs[key] = formattedVal
		}
	}

	configsValue, diags := types.MapValueFrom(ctx, types.StringType, stringConfigs)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	data.Configs = configsValue

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *MgrModuleConfigResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data MgrModuleConfigResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	moduleName := data.ModuleName.ValueString()

	var currentConfigsMap map[string]string
	resp.Diagnostics.Append(data.Configs.ElementsAs(ctx, &currentConfigsMap, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	options, err := r.client.MgrGetModuleOptions(ctx, moduleName)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to get module options for '%s': %s", moduleName, err),
		)
		return
	}

	defaultConfigs := make(CephAPIMgrModuleConfig)
	for key := range currentConfigsMap {
		if option, ok := options[key]; ok {
			defaultConfigs[key] = option.DefaultValue
		}
	}

	if len(defaultConfigs) > 0 {
		err = r.client.MgrSetModuleConfig(ctx, moduleName, defaultConfigs)
		if err != nil {
			resp.Diagnostics.AddError(
				"API Request Error",
				fmt.Sprintf("Unable to reset MGR module config for '%s' to defaults: %s", moduleName, err),
			)
			return
		}
	}
}

func (r *MgrModuleConfigResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	moduleName := req.ID

	readConfigs, err := r.client.MgrGetModuleConfig(ctx, moduleName)
	if err != nil {
		resp.Diagnostics.AddError(
			"Import Error",
			fmt.Sprintf("Unable to read MGR module config for '%s': %s", moduleName, err),
		)
		return
	}

	options, err := r.client.MgrGetModuleOptions(ctx, moduleName)
	if err != nil {
		resp.Diagnostics.AddError(
			"Import Error",
			fmt.Sprintf("Unable to get module options for '%s': %s", moduleName, err),
		)
		return
	}

	stringConfigs := make(map[string]string)
	for key, val := range readConfigs {
		option, ok := options[key]
		if !ok {
			continue
		}

		valStr, err := formatMgrModuleConfigValue(val)
		if err != nil {
			resp.Diagnostics.AddError(
				"Configuration Value Formatting Error",
				fmt.Sprintf("Unable to format config value for key '%s': %s", key, err),
			)
			return
		}

		defaultStr, err := formatMgrModuleConfigValue(option.DefaultValue)
		if err != nil {
			resp.Diagnostics.AddError(
				"Configuration Value Formatting Error",
				fmt.Sprintf("Unable to format default value for key '%s': %s", key, err),
			)
			return
		}

		if valStr != defaultStr {
			stringConfigs[key] = valStr
		}
	}

	configsValue, diags := types.MapValueFrom(ctx, types.StringType, stringConfigs)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("module_name"), moduleName)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("configs"), configsValue)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), moduleName)...)
}
