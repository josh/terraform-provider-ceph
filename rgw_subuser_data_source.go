package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dataSourceSchema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &RGWSubuserDataSource{}

func newRGWSubuserDataSource() datasource.DataSource {
	return &RGWSubuserDataSource{}
}

type RGWSubuserDataSource struct {
	client *CephAPIClient
}

type RGWSubuserDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Permissions types.String `tfsdk:"permissions"`
}

func (d *RGWSubuserDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rgw_subuser"
}

func (d *RGWSubuserDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dataSourceSchema.Schema{
		MarkdownDescription: "This data source allows you to get information about a Ceph RGW subuser.",
		Attributes: map[string]dataSourceSchema.Attribute{
			"id": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The subuser ID in the format 'parent_user:subuser'",
				Required:            true,
			},
			"permissions": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The permissions assigned to the subuser (read, write, readwrite, or full-control)",
				Computed:            true,
			},
		},
	}
}

func (d *RGWSubuserDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*CephAPIClient)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *CephAPIClient, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	d.client = client
}

func (d *RGWSubuserDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data RGWSubuserDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	subuserID := data.ID.ValueString()

	parts := strings.SplitN(subuserID, ":", 2)
	if len(parts) != 2 {
		resp.Diagnostics.AddError(
			"Invalid Subuser ID",
			fmt.Sprintf("Subuser ID must be in the format 'parent_user:subuser', got: %s", subuserID),
		)
		return
	}
	parentUID := parts[0]

	user, err := d.client.RGWGetUser(ctx, parentUID)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to get RGW user from Ceph API: %s", err),
		)
		return
	}

	var foundSubuser *CephAPIRGWSubuser
	for i := range user.Subusers {
		if user.Subusers[i].ID == subuserID {
			foundSubuser = &user.Subusers[i]
			break
		}
	}

	if foundSubuser == nil {
		resp.Diagnostics.AddError(
			"Subuser Not Found",
			fmt.Sprintf("Subuser %s not found for user %s", subuserID, parentUID),
		)
		return
	}

	data.Permissions = types.StringValue(foundSubuser.Permissions)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
