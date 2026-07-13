package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceSchema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/josh/terraform-provider-ceph/internal/cephvalues"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var (
	_ resource.Resource                = &DashboardSettingResource{}
	_ resource.ResourceWithImportState = &DashboardSettingResource{}
)

func newDashboardSettingResource() resource.Resource {
	return &DashboardSettingResource{}
}

type DashboardSettingResource struct {
	client *restapi.Client
}

type DashboardSettingResourceModel struct {
	Name    types.String `tfsdk:"name"`
	Value   types.String `tfsdk:"value"`
	Type    types.String `tfsdk:"type"`
	Default types.String `tfsdk:"default"`
}

func (r *DashboardSettingResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dashboard_setting"
}

func (r *DashboardSettingResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceSchema.Schema{
		MarkdownDescription: "This resource manages a Ceph dashboard setting (e.g. `GRAFANA_API_URL`), distinct from cluster configuration which is managed by `ceph_config`. Settings always exist with a default value; creating this resource sets the value and destroying it resets the setting to its default.",
		Attributes: map[string]resourceSchema.Attribute{
			"name": resourceSchema.StringAttribute{
				MarkdownDescription: "The name of the dashboard setting.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"value": resourceSchema.StringAttribute{
				MarkdownDescription: "The value of the setting as a string. The dashboard converts it to the setting's native type (bool, int or string).",
				Required:            true,
			},
			"type": resourceSchema.StringAttribute{
				MarkdownDescription: "The native type of the setting.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"default": resourceSchema.StringAttribute{
				MarkdownDescription: "The default value of the setting.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *DashboardSettingResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*restapi.Client)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *restapi.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = client
}

func (r *DashboardSettingResource) updateModelFromAPI(data *DashboardSettingResourceModel, setting *restapi.DashboardSetting) error {
	value, err := cephvalues.MgrModuleString(data.Value.ValueString(), setting.Value)
	if err != nil {
		return fmt.Errorf("unable to format setting value: %w", err)
	}
	defaultValue, err := cephvalues.FormatMgrModuleValue(setting.Default)
	if err != nil {
		return fmt.Errorf("unable to format setting default: %w", err)
	}

	data.Value = types.StringValue(value)
	data.Type = types.StringValue(setting.Type)
	data.Default = types.StringValue(defaultValue)
	return nil
}

func (r *DashboardSettingResource) setAndRead(ctx context.Context, data *DashboardSettingResourceModel) error {
	name := data.Name.ValueString()

	err := r.client.SetDashboardSetting(ctx, name, data.Value.ValueString())
	if err != nil {
		return fmt.Errorf("unable to set dashboard setting '%s': %w", name, err)
	}

	// Setting an unknown name silently succeeds, so the read back is what
	// validates the name actually exists.
	setting, err := r.client.GetDashboardSetting(ctx, name)
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			return fmt.Errorf("dashboard setting '%s' does not exist", name)
		}
		return fmt.Errorf("unable to read dashboard setting '%s' after setting it: %w", name, err)
	}

	return r.updateModelFromAPI(data, setting)
}

func (r *DashboardSettingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data DashboardSettingResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.setAndRead(ctx, &data); err != nil {
		resp.Diagnostics.AddError("API Request Error", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DashboardSettingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data DashboardSettingResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	setting, err := r.client.GetDashboardSetting(ctx, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read dashboard setting '%s': %s", data.Name.ValueString(), err),
		)
		return
	}

	if err := r.updateModelFromAPI(&data, setting); err != nil {
		resp.Diagnostics.AddError("API Response Error", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DashboardSettingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data DashboardSettingResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.setAndRead(ctx, &data); err != nil {
		resp.Diagnostics.AddError("API Request Error", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DashboardSettingResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data DashboardSettingResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteDashboardSetting(ctx, data.Name.ValueString())
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to reset dashboard setting '%s': %s", data.Name.ValueString(), err),
		)
		return
	}
}

func (r *DashboardSettingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}
